package tracenetsoftirq

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/util"
	log "github.com/sirupsen/logrus"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_softirq_event_t bpf ../../../../bpf/net_softirq.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86
const (
	NETSOFTIRQ_SCHED_SLOW   = "net_softirq_schedslow"       //nolint
	NETSOFTIRQ_SCHED_100MS  = "net_softirq_schedslow100ms"  //nolint
	NETSOFTIRQ_EXCUTE_SLOW  = "net_softirq_excuteslow"      //nolint
	NETSOFTIRQ_EXCUTE_100MS = "net_softirq_excuteslow100ms" //nolint
)

var (
	metrics          = []string{NETSOFTIRQ_SCHED_SLOW, NETSOFTIRQ_SCHED_100MS, NETSOFTIRQ_EXCUTE_SLOW, NETSOFTIRQ_EXCUTE_100MS}
	probeName        = "netsoftirq"
	_netSoftirqProbe = &netSoftirqProbe{}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}

func metricsProbeCreator(_ map[string]interface{}) (probe.MetricsProbe, error) {
	p := &metricsProbe{}
	batchMetrics := probe.NewLegacyBatchMetrics(probeName, metrics, p.CollectOnce)

	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

func eventProbeCreator(sink chan<- *probe.Event, _ map[string]interface{}) (probe.EventProbe, error) {
	p := &eventProbe{
		sink: sink,
	}
	return probe.NewEventProbe(probeName, p), nil
}

type metricsProbe struct {
}

func (p *metricsProbe) Start(_ context.Context) error {
	return _netSoftirqProbe.start(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return _netSoftirqProbe.stop(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) CollectOnce() (map[string]map[uint32]uint64, error) {
	return _netSoftirqProbe.copyMetricsMap(), nil
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(_ context.Context) error {
	err := _netSoftirqProbe.start(probe.ProbeTypeEvent)
	if err != nil {
		return err
	}

	_netSoftirqProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(_ context.Context) error {
	return _netSoftirqProbe.stop(probe.ProbeTypeEvent)
}

type netSoftirqProbe struct {
	objs        bpfObjects
	links       []link.Link
	sink        chan<- *probe.Event
	refcnt      [probe.ProbeTypeCount]int
	lock        sync.Mutex
	perfReader  *perf.Reader
	metricsMap  map[string]map[uint32]uint64
	metricsLock sync.RWMutex
}

func (p *netSoftirqProbe) stop(probeType probe.Type) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.refcnt[probeType] == 0 {
		return fmt.Errorf("probe %s never start", probeType)
	}

	p.refcnt[probeType]--
	if p.totalReferenceCountLocked() == 0 {
		return p.cleanup()
	}
	return nil
}

func (p *netSoftirqProbe) cleanup() error {
	if p.perfReader != nil {
		p.perfReader.Close()
	}

	for _, link := range p.links {
		link.Close()
	}

	p.links = nil

	p.objs.Close()

	return nil
}

func (p *netSoftirqProbe) copyMetricsMap() map[string]map[uint32]uint64 {
	p.metricsLock.RLock()
	defer p.metricsLock.RUnlock()
	return probe.CopyLegacyMetricsMap(p.metricsMap)
}

func (p *netSoftirqProbe) totalReferenceCountLocked() int {
	var c int
	for _, n := range p.refcnt {
		c += n
	}
	return c
}

func (p *netSoftirqProbe) start(probeType probe.Type) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.refcnt[probeType]++
	if p.totalReferenceCountLocked() == 1 {
		if err = p.loadAndAttachBPF(); err != nil {
			log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
			_ = p.cleanup()
			return fmt.Errorf("%s failed load bpf: %w", probeName, err)
		}

		// 初始化map的读接口
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspSoftirqEvents, int(unsafe.Sizeof(bpfInspSoftirqEventT{})))
		if err != nil {
			log.Errorf("%s failed create perf reader, err: %v", probeName, err)
			return err
		}

		go p.perfLoop()
	}

	return nil
}

func (p *netSoftirqProbe) updateMetrics(metrics string) {
	p.metricsLock.Lock()
	defer p.metricsLock.Unlock()
	if _, ok := p.metricsMap[metrics]; !ok {
		p.metricsMap[metrics] = make(map[uint32]uint64)
	}

	p.metricsMap[metrics][0]++
}

func (p *netSoftirqProbe) perfLoop() {
	for {
		record, err := p.perfReader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				log.Errorf("%s received signal, exiting..", probeName)
				return
			}
			log.Warnf("%s failed reading from reader, err: %v", probeName, err)
			continue
		}

		if record.LostSamples != 0 {
			log.Warnf("%s perf event ring buffer full, drop: %d", probeName, record.LostSamples)
			continue
		}

		var event bpfInspSoftirqEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Errorf("%s failed parsing event, err: %v", probeName, err)
			continue
		}

		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
		}

		/*
					#define PHASE_SCHED 1
			        #define PHASE_EXCUTE 2
		*/
		switch event.Phase {
		case 1:
			if event.Latency > 100000000 {
				evt.Type = "NETSOFTIRQ_SCHED_100MS"
				p.updateMetrics(NETSOFTIRQ_SCHED_100MS)
			} else {
				evt.Type = "NETSOFTIRQ_SCHED_SLOW"
				p.updateMetrics(NETSOFTIRQ_SCHED_SLOW)
			}
		case 2:
			if event.Latency > 100000000 {
				evt.Type = "NETSOFTIRQ_EXCUTE_100MS"
				p.updateMetrics(NETSOFTIRQ_EXCUTE_100MS)
			} else {
				evt.Type = "NETSOFTIRQ_EXCUTE_SLOW"
				p.updateMetrics(NETSOFTIRQ_EXCUTE_SLOW)
			}

		default:
			log.Infof("%s failed parsing event, phase: %d", probeName, event.Phase)
			continue
		}

		evt.Message = fmt.Sprintf("cpu=%d pid=%d latency=%s ", event.Cpu, event.Pid, bpfutil.GetHumanTimes(event.Latency))
		if p.sink != nil {
			log.Debugf("%s sink event %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}
}

func (p *netSoftirqProbe) loadAndAttachBPF() error {
	// 准备动作
	if err := rlimit.RemoveMemlock(); err != nil {
		return err
	}

	opts := ebpf.CollectionOptions{}
	// 获取btf信息
	opts.Programs = ebpf.ProgramOptions{
		KernelTypes: bpfutil.LoadBTFSpecOrNil(),
	}

	// 获取Loaded的程序/map的fd信息
	if err := loadBpfObjects(&p.objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %v", err)
	}

	prograise, err := link.Tracepoint("irq", "softirq_raise", p.objs.TraceSoftirqRaise, &link.TracepointOptions{})
	if err != nil {
		return fmt.Errorf("link softirq_raise: %s", err.Error())
	}
	p.links = append(p.links, prograise)

	progentry, err := link.Tracepoint("irq", "softirq_entry", p.objs.TraceSoftirqEntry, &link.TracepointOptions{})
	if err != nil {
		return fmt.Errorf("link softirq_entry: %s", err.Error())
	}
	p.links = append(p.links, progentry)

	progexit, err := link.Tracepoint("irq", "softirq_exit", p.objs.TraceSoftirqExit, &link.TracepointOptions{})
	if err != nil {
		return fmt.Errorf("link softirq_exit: %w", err)
	}
	p.links = append(p.links, progexit)

	return nil
}
