package tracesoftirq

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_softirq_event_t bpf ../../../../bpf/softirq.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86
const (
	SOFTIRQ_SCHED_SLOW   = "schedslow"       //nolint
	SOFTIRQ_SCHED_100MS  = "schedslow100ms"  //nolint
	SOFTIRQ_EXCUTE_SLOW  = "excuteslow"      //nolint
	SOFTIRQ_EXCUTE_100MS = "excuteslow100ms" //nolint
)

var (
	probeName     = "softirq"
	softirqTypes  = []string{"hi", "timer", "net_tx", "net_rx", "block", "irq_poll", "tasklet", "sched", "hrtimer", "rcu"}
	_softirqProbe = &softirqProbe{
		metricsMap: map[string]map[string]uint64{
			SOFTIRQ_SCHED_SLOW:   {},
			SOFTIRQ_SCHED_100MS:  {},
			SOFTIRQ_EXCUTE_SLOW:  {},
			SOFTIRQ_EXCUTE_100MS: {},
		},
	}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}

type softirqArgs struct {
	SoftirqTypes []string `mapstructure:"softirq-types"`
}

func metricsProbeCreator(args softirqArgs) (probe.MetricsProbe, error) {
	if len(args.SoftirqTypes) == 0 {
		args.SoftirqTypes = []string{"net_rx"}
	}
	_softirqProbe.metricsProbeIrqTypes = softirqTypesBits(args.SoftirqTypes)

	p := &metricsProbe{}
	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: []string{"k8s_node", "softirq_type"},
		SingleMetricsOpts: []probe.SingleMetricsOpts{
			{Name: SOFTIRQ_SCHED_SLOW, ValueType: prometheus.CounterValue},
			{Name: SOFTIRQ_SCHED_100MS, ValueType: prometheus.CounterValue},
			{Name: SOFTIRQ_EXCUTE_SLOW, ValueType: prometheus.CounterValue},
			{Name: SOFTIRQ_EXCUTE_100MS, ValueType: prometheus.CounterValue},
		},
	}
	batchMetrics := probe.NewBatchMetrics(opts, p.collectOnce)
	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

func eventProbeCreator(sink chan<- *probe.Event, args softirqArgs) (probe.EventProbe, error) {
	if len(args.SoftirqTypes) == 0 {
		args.SoftirqTypes = []string{"net_rx"}
	}
	_softirqProbe.eventProbeIrqTypes = softirqTypesBits(args.SoftirqTypes)
	p := &eventProbe{
		sink: sink,
	}
	return probe.NewEventProbe(probeName, p), nil
}

type metricsProbe struct {
}

func (p *metricsProbe) Start(_ context.Context) error {
	return _softirqProbe.start(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return _softirqProbe.stop(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) collectOnce(emit probe.Emit) error {
	_softirqProbe.metricsLock.RLock()
	defer _softirqProbe.metricsLock.RUnlock()
	nodeName := nettop.GetNodeName()
	for metricsName, values := range _softirqProbe.metricsMap {
		for _, irqType := range enabledIrqTypes(_softirqProbe.metricsProbeIrqTypes) {
			emit(metricsName, []string{nodeName, irqType}, float64(values[irqType]))
		}
	}
	return nil
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(_ context.Context) error {
	err := _softirqProbe.start(probe.ProbeTypeEvent)
	if err != nil {
		return err
	}

	_softirqProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(_ context.Context) error {
	return _softirqProbe.stop(probe.ProbeTypeEvent)
}

type softirqProbe struct {
	objs                 bpfObjects
	links                []link.Link
	sink                 chan<- *probe.Event
	refcnt               [probe.ProbeTypeCount]int
	metricsProbeIrqTypes uint32
	eventProbeIrqTypes   uint32
	ebpfProbeIrqType     uint32
	lock                 sync.Mutex
	perfReader           *perf.Reader
	metricsMap           map[string]map[string]uint64
	metricsLock          sync.RWMutex
}

func (p *softirqProbe) stop(probeType probe.Type) error {
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

func (p *softirqProbe) cleanup() error {
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

func (p *softirqProbe) totalReferenceCountLocked() int {
	var c int
	for _, n := range p.refcnt {
		c += n
	}
	return c
}

func (p *softirqProbe) start(probeType probe.Type) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.refcnt[probeType]++

	var needLoad bool
	if p.totalReferenceCountLocked() == 1 {
		needLoad = true
	} else if p.ebpfProbeIrqType != p.metricsProbeIrqTypes|p.eventProbeIrqTypes {
		_ = p.cleanup()
		needLoad = true
	}

	if needLoad {
		p.ebpfProbeIrqType = p.metricsProbeIrqTypes | p.eventProbeIrqTypes
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

func (p *softirqProbe) updateMetrics(metrics string, irqType uint32) {
	p.metricsLock.Lock()
	defer p.metricsLock.Unlock()
	if !filterIrqEvent(p.metricsProbeIrqTypes, irqType) {
		return
	}
	if _, ok := p.metricsMap[metrics]; !ok {
		p.metricsMap[metrics] = make(map[string]uint64)
	}

	p.metricsMap[metrics][convertIrqType(irqType)]++
}

func (p *softirqProbe) perfLoop() {
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
			Labels: []probe.Label{
				{
					Name:  "type",
					Value: convertIrqType(event.VecNr),
				},
			},
		}

		/*
					#define PHASE_SCHED 1
			        #define PHASE_EXCUTE 2
		*/
		switch event.Phase {
		case 1:
			if event.Latency > 100000000 {
				evt.Type = "SOFTIRQ_SCHED_100MS"
				p.updateMetrics(SOFTIRQ_SCHED_100MS, event.VecNr)
			} else {
				evt.Type = "SOFTIRQ_SCHED_SLOW"
				p.updateMetrics(SOFTIRQ_SCHED_SLOW, event.VecNr)
			}
		case 2:
			if event.Latency > 100000000 {
				evt.Type = "SOFTIRQ_EXCUTE_100MS"
				p.updateMetrics(SOFTIRQ_EXCUTE_100MS, event.VecNr)
			} else {
				evt.Type = "SOFTIRQ_EXCUTE_SLOW"
				p.updateMetrics(SOFTIRQ_EXCUTE_SLOW, event.VecNr)
			}

		default:
			log.Infof("%s failed parsing event, phase: %d", probeName, event.Phase)
			continue
		}

		evt.Message = fmt.Sprintf("cpu=%d pid=%d latency=%s ", event.Cpu, event.Pid, bpfutil.GetHumanTimes(event.Latency))
		if filterIrqEvent(p.eventProbeIrqTypes, event.VecNr) && p.sink != nil {
			log.Debugf("%s sink event %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}
}

func (p *softirqProbe) loadAndAttachBPF() error {
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
	spec, err := loadBpf()
	if err != nil {
		return err
	}
	err = spec.RewriteConstants(map[string]interface{}{"irq_filter_bits": p.ebpfProbeIrqType})
	if err != nil {
		return err
	}

	err = spec.LoadAndAssign(&p.objs, &opts)
	if err != nil {
		return nil
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

func convertIrqType(vec uint32) string {
	return softirqTypes[vec]
}

func filterIrqEvent(filterBits, vec uint32) bool {
	return filterBits&(1<<vec) != 0
}

func enabledIrqTypes(filterBits uint32) []string {
	var types []string
	for i, bit := range softirqTypes {
		if filterBits&(1<<i) != 0 {
			types = append(types, bit)
		}
	}
	return types
}

func softirqTypesBits(types []string) uint32 {
	var bits uint32
	for _, softirqType := range types {
		index := lo.IndexOf(softirqTypes, softirqType)
		if index >= 0 {
			bits |= 1 << index
		}
	}
	return bits
}
