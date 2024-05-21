package tracevirtcmdlat

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target=${GOARCH} -cc clang -cflags $BPF_CFLAGS -type insp_virtcmdlat_event_t  bpf ../../../../bpf/virtcmdlatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_${GOARCH}

const (
	VIRTCMD100MS       = "latency100ms"
	VIRTCMD            = "latency"
	VIRTCMDEXCUTE      = "VIRTCMDEXCUTE"
	VIRTCMDEXCUTE100MS = "VIRTCMDEXCUTE_100MS"

	fn        = "virtnet_send_command"
	probeName = "virtcmdlatency"
)

var (
	metrics              = []string{VIRTCMD100MS, VIRTCMD}
	_virtcmdLatencyProbe = &virtcmdLatencyProbe{
		metricsMap: map[string]map[uint32]uint64{
			VIRTCMD:      {0: 0},
			VIRTCMD100MS: {0: 0},
		},
	}
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

func eventProbeCreator(sink chan<- *probe.Event) (probe.EventProbe, error) {
	p := &eventProbe{
		sink: sink,
	}
	return probe.NewEventProbe(probeName, p), nil
}

type metricsProbe struct {
}

func (p *metricsProbe) Start(ctx context.Context) error {
	return _virtcmdLatencyProbe.start(ctx, probe.ProbeTypeMetrics)
}

func (p *metricsProbe) Stop(ctx context.Context) error {
	return _virtcmdLatencyProbe.stop(ctx, probe.ProbeTypeMetrics)
}

func (p *metricsProbe) CollectOnce() (map[string]map[uint32]uint64, error) {
	return _virtcmdLatencyProbe.copyMetricsMap(), nil
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(ctx context.Context) error {
	err := _virtcmdLatencyProbe.start(ctx, probe.ProbeTypeEvent)
	if err != nil {
		return err
	}

	_virtcmdLatencyProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(ctx context.Context) error {
	return _virtcmdLatencyProbe.stop(ctx, probe.ProbeTypeEvent)
}

type virtcmdLatencyProbe struct {
	objs        bpfObjects
	links       []link.Link
	sink        chan<- *probe.Event
	refcnt      [probe.ProbeTypeCount]int
	lock        sync.Mutex
	perfReader  *perf.Reader
	metricsMap  map[string]map[uint32]uint64
	metricsLock sync.RWMutex
}

func (p *virtcmdLatencyProbe) stop(_ context.Context, probeType probe.Type) error {
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

func (p *virtcmdLatencyProbe) cleanup() error {
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

func (p *virtcmdLatencyProbe) updateMetrics(metrics string) {
	p.metricsLock.Lock()
	defer p.metricsLock.Unlock()
	if _, ok := p.metricsMap[metrics]; !ok {
		p.metricsMap[metrics] = make(map[uint32]uint64)
	}

	p.metricsMap[metrics][0]++
}

func (p *virtcmdLatencyProbe) copyMetricsMap() map[string]map[uint32]uint64 {
	p.metricsLock.RLock()
	defer p.metricsLock.RUnlock()
	return probe.CopyLegacyMetricsMap(p.metricsMap)
}

func (p *virtcmdLatencyProbe) totalReferenceCountLocked() int {
	var c int
	for _, n := range p.refcnt {
		c += n
	}
	return c
}

func (p *virtcmdLatencyProbe) start(_ context.Context, probeType probe.Type) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.refcnt[probeType]++
	if p.totalReferenceCountLocked() == 1 {
		if err = p.loadAndAttachBPF(); err != nil {
			log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
			_ = p.cleanup()
			return
		}
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspVirtcmdlatEvents, int(unsafe.Sizeof(bpfInspVirtcmdlatEventT{})))
		if err != nil {
			log.Warnf("%s failed create new perf reader, err: %v", probeName, err)
			return
		}

		go p.perfLoop()
	}

	return nil
}

func (p *virtcmdLatencyProbe) perfLoop() {
	for {
		record, err := p.perfReader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				log.Infof("%s received signal, exiting..", probeName)
				return
			}
			log.Infof("%s failed reading from reader, err: %v", probeName, err)
			continue
		}

		if record.LostSamples != 0 {
			log.Infof("%s perf event ring buffer full, drop: %d", probeName, record.LostSamples)
			continue
		}

		var event bpfInspVirtcmdlatEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Infof("%s failed parsing event, err: %v", probeName, err)
			continue
		}

		evt := &probe.Event{
			Type: VIRTCMDEXCUTE,
		}

		p.updateMetrics(VIRTCMD)
		if event.Latency > 100_000_000 {
			evt.Type = VIRTCMDEXCUTE100MS
			p.updateMetrics(VIRTCMD100MS)
		}

		evt.Message = fmt.Sprintf("cpu=%d  pid=%d  latency=%s", event.Cpu, event.Pid, bpfutil.GetHumanTimes(event.Latency))
		if p.sink != nil {
			log.Debugf("%s sink event %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}
}

func (p *virtcmdLatencyProbe) loadAndAttachBPF() error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove limit failed: %s", err.Error())
	}

	opts := ebpf.CollectionOptions{}

	opts.Programs = ebpf.ProgramOptions{
		KernelTypes: bpfutil.LoadBTFSpecOrNil(),
	}

	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&p.objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %s", err.Error())
	}

	linkentry, err := link.Kprobe(fn, p.objs.TraceVirtcmd, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link %s: %s", fn, err.Error())
	}
	p.links = append(p.links, linkentry)

	linkexit, err := link.Kretprobe(fn, p.objs.TraceVirtcmdret, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link ret %s: %s", fn, err.Error())
	}

	p.links = append(p.links, linkexit)
	return nil
}
