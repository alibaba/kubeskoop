package tracepacketloss

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	log "github.com/sirupsen/logrus"

	"strings"
	"sync"
	"unsafe"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_pl_event_t -type addr -type tuple bpf ../../../../bpf/packetloss.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	packetLossTotal     = "total"
	packetLossNetfilter = "netfilter"
	//PACKETLOSS_ABNORMAL  = "abnormal"
	//PACKETLOSS_TCPSTATEM = "tcpstatm"
	//PACKETLOSS_TCPRCV    = "tcprcv"
	//PACKETLOSS_TCPHANDLE = "tcphandle"

	PacketLoss = "PacketLoss"
)

var (
	ignoreSymbolList = map[string]struct{}{}
	uselessSymbols   = map[string]bool{
		"sk_stream_kill_queues": true,
		"unix_release_sock":     true,
	}

	netfilterSymbol = "nf_hook_slow"
	//tcpstatmSymbol  = "tcp_rcv_state_process"
	//tcprcvSymbol    = "tcp_v4_rcv"
	//tcpdorcvSymbol  = "tcp_v4_do_rcv"

	probeName = "packetloss"

	_packetLossProbe = &packetLossProbe{}
)

func init() {
	var err error
	_packetLossProbe.cache, err = lru.New[probe.Tuple, *Counter](102400)
	if err != nil {
		panic(fmt.Sprintf("cannot create lru cache for packetloss probe:%v", err))
	}
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}

func metricsProbeCreator() (probe.MetricsProbe, error) {
	p := &metricsProbe{}

	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: probe.TupleMetricsLabels,
		SingleMetricsOpts: []probe.SingleMetricsOpts{
			{Name: packetLossTotal, ValueType: prometheus.CounterValue},
			{Name: packetLossNetfilter, ValueType: prometheus.CounterValue},
		},
	}
	batchMetrics := probe.NewBatchMetrics(opts, p.collectOnce)
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

func (p *metricsProbe) Start(ctx context.Context) error {
	return _packetLossProbe.start(ctx, probe.ProbeTypeMetrics)
}

func (p *metricsProbe) Stop(ctx context.Context) error {
	return _packetLossProbe.stop(ctx, probe.ProbeTypeMetrics)
}

func (p *metricsProbe) collectOnce(emit probe.Emit) error {
	keys := _packetLossProbe.cache.Keys()
	for _, tuple := range keys {
		counter, ok := _packetLossProbe.cache.Get(tuple)
		if !ok || counter == nil {
			continue
		}

		labels := probe.BuildTupleMetricsLabels(&tuple)
		emit(packetLossTotal, labels, float64(counter.Total))
		emit(packetLossNetfilter, labels, float64(counter.Netfilter))
	}
	return nil
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(ctx context.Context) error {
	err := _packetLossProbe.start(ctx, probe.ProbeTypeEvent)
	if err != nil {
		return err
	}

	_packetLossProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(ctx context.Context) error {
	return _packetLossProbe.stop(ctx, probe.ProbeTypeEvent)
}

type Counter struct {
	Total     uint32
	Netfilter uint32
}

type packetLossProbe struct {
	objs       bpfObjects
	links      []link.Link
	sink       chan<- *probe.Event
	refcnt     [probe.ProbeTypeCount]int
	lock       sync.Mutex
	perfReader *perf.Reader

	cache *lru.Cache[probe.Tuple, *Counter]
}

func (p *packetLossProbe) stop(_ context.Context, probeType probe.Type) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.refcnt[probeType] == 0 {
		return fmt.Errorf("probe %s never start", probeType)
	}

	p.refcnt[probeType]--

	if p.refcnt[probe.ProbeTypeEvent] == 0 {
		if p.perfReader != nil {
			p.perfReader.Close()
		}
	}

	if p.totalReferenceCountLocked() == 0 {
		return p.cleanup()
	}
	return nil
}

func (p *packetLossProbe) cleanup() error {
	for _, link := range p.links {
		link.Close()
	}

	p.links = nil

	p.objs.Close()

	return nil
}

func (p *packetLossProbe) totalReferenceCountLocked() int {
	var c int
	for _, n := range p.refcnt {
		c += n
	}
	return c
}

func (p *packetLossProbe) start(ctx context.Context, probeType probe.Type) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.refcnt[probeType] != 0 {
		return fmt.Errorf("%s(%s) has already started", probeName, probeType)
	}

	p.refcnt[probeType]++
	if p.totalReferenceCountLocked() == 1 {
		if err = p.loadAndAttachBPF(); err != nil {
			log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
			_ = p.cleanup()
			return
		}
	}

	if p.refcnt[probe.ProbeTypeEvent] == 1 {
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspPlEvent, int(unsafe.Sizeof(bpfInspPlEventT{})))
		if err != nil {
			log.Errorf("%s error create perf reader, err: %v", probeName, err)
			_ = p.stop(ctx, probeType)
			return
		}

		go p.perfLoop()
	}
	return nil
}

func (p *packetLossProbe) loadAndAttachBPF() error {
	// Allow the current process to lock memory for eBPF resources.
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

	pl, err := link.Tracepoint("skb", "kfree_skb", p.objs.KfreeSkb, &link.TracepointOptions{})
	if err != nil {
		return fmt.Errorf("link tracepoint kfree_skb failed: %s", err.Error())
	}
	p.links = append(p.links, pl)
	return nil
}

func ignoreLocation(loc uint64) bool {
	sym, err := bpfutil.GetSymPtFromBpfLocation(loc)
	if err != nil {
		log.Infof("cannot find location %d", loc)
		// emit the event anyway
		return false
	}

	_, ok := uselessSymbols[sym.GetName()]
	return !ok
}

func toProbeTuple(t *bpfTuple) *probe.Tuple {
	return &probe.Tuple{
		Protocol: t.L4Proto,
		Src:      bpfutil.GetAddrStr(t.L3Proto, t.Saddr.V6addr),
		Dst:      bpfutil.GetAddrStr(t.L3Proto, t.Daddr.V6addr),
		Sport:    t.Sport,
		Dport:    t.Dport,
	}
}

func (p *packetLossProbe) countByLocation(loc uint64, counter *Counter) {
	sym, err := bpfutil.GetSymPtFromBpfLocation(loc)
	if err != nil {
		log.Warnf("%s get sym failed, location: %x, err: %v", probeName, loc, err)
		return
	}

	switch sym.GetName() {
	case netfilterSymbol:
		counter.Netfilter++
	}

}

func (p *packetLossProbe) perfLoop() {
	for {
	anotherLoop:
		record, err := p.perfReader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				log.Infof("%s received signal, exiting..", probeName)
				return
			}
			log.Errorf("%s failed reading from reader, err: %v", probeName, err)
			continue
		}

		if record.LostSamples != 0 {
			log.Warnf("%s perf event ring buffer full, drop: %d", probeName, record.LostSamples)
			continue
		}

		var event bpfInspPlEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.NativeEndian, &event); err != nil {
			log.Errorf("%s failed parsing event, err: %v", probeName, err)
			continue
		}

		if ignoreLocation(event.Location) {
			continue
		}

		tuple := toProbeTuple(&event.Tuple)

		v, ok := p.cache.Get(*tuple)
		if !ok {
			v = &Counter{}
			p.cache.Add(*tuple, v)
		}
		v.Total++
		p.countByLocation(event.Location, v)

		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Type:      PacketLoss,
			Labels:    probe.BuildTupleEventLabels(tuple),
		}

		//TODO add trigger to enable/disable stack
		stacks, err := bpfutil.GetSymsByStack(uint32(event.StackId), p.objs.InspPlStack)
		if err != nil {
			log.Warnf("%s failed get sym by stack, err: %v", probeName, err)
			continue
		}
		var strs []string
		for _, sym := range stacks {
			if _, ok := ignoreSymbolList[sym.GetName()]; ok {
				goto anotherLoop
			}
			strs = append(strs, sym.GetExpr())
		}

		evt.Message = strings.Join(strs, "\n")

		if p.sink != nil {
			p.sink <- evt
		}
	}
}
