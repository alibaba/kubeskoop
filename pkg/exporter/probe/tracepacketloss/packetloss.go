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

	featureSwitchEnablePacketLossStackKey = 0
)

var (
	ignoreSymbolList = map[string]struct{}{}
	uselessSymbols   = map[string]bool{
		"sk_stream_kill_queues": true,
		"unix_release_sock":     true,
		"nfnetlink_rcv_batch":   true,
		"skb_queue_purge":       true,
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

type packetlossArgs struct {
	EnableStack bool `mapstructure:"EnableStack"`
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

func eventProbeCreator(sink chan<- *probe.Event, args packetlossArgs) (probe.EventProbe, error) {
	p := &eventProbe{
		args: args,
		sink: sink,
	}
	return probe.NewEventProbe(probeName, p), nil
}

type metricsProbe struct {
}

func (p *metricsProbe) Start(_ context.Context) error {
	cfg := probeConfig{}
	return _packetLossProbe.start(probe.ProbeTypeMetrics, &cfg)
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return _packetLossProbe.stop(probe.ProbeTypeMetrics)
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
	args packetlossArgs
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(_ context.Context) error {
	cfg := probeConfig{
		enableStack: e.args.EnableStack,
	}
	err := _packetLossProbe.start(probe.ProbeTypeEvent, &cfg)
	if err != nil {
		return err
	}

	_packetLossProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(_ context.Context) error {
	return _packetLossProbe.stop(probe.ProbeTypeEvent)
}

type Counter struct {
	Total     uint32
	Netfilter uint32
}

type probeConfig struct {
	enableStack bool
}

type packetLossProbe struct {
	objs        bpfObjects
	links       []link.Link
	sink        chan<- *probe.Event
	probeConfig [probe.ProbeTypeCount]*probeConfig
	lock        sync.Mutex
	perfReader  *perf.Reader

	cache *lru.Cache[probe.Tuple, *Counter]
}

func (p *packetLossProbe) probeCount() int {
	var ret int
	for _, cfg := range p.probeConfig {
		if cfg != nil {
			ret++
		}
	}
	return ret
}

func (p *packetLossProbe) stop(probeType probe.Type) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.probeConfig[probeType] == nil {
		return fmt.Errorf("probe %s never start", probeType)
	}

	p.probeConfig[probeType] = nil

	if probeType == probe.ProbeTypeEvent {
		p.closePerfReader()
	}

	if p.probeCount() == 0 {
		p.cleanup()
	}

	return nil
}

func (p *packetLossProbe) closePerfReader() {
	if p.perfReader != nil {
		p.perfReader.Close()
		p.perfReader = nil
	}
}

func (p *packetLossProbe) cleanup() {
	for _, link := range p.links {
		link.Close()
	}

	p.links = nil

	p.objs.Close()

}

func (p *packetLossProbe) start(probeType probe.Type, cfg *probeConfig) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.probeConfig[probeType] != nil {
		return fmt.Errorf("%s(%s) has already started", probeName, probeType)
	}

	p.probeConfig[probeType] = cfg

	if err := p.reinstallBPFLocked(); err != nil {
		return fmt.Errorf("%s failed install ebpf: %w", probeName, err)
	}

	var err error

	if probeType == probe.ProbeTypeEvent {
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspPlEvent, int(unsafe.Sizeof(bpfInspPlEventT{})))
		if err != nil {
			log.Errorf("%s error create perf reader, err: %v", probeName, err)
			return err
		}

		go p.perfLoop()
	}

	return nil
}

func (p *packetLossProbe) reinstallBPFLocked() error {
	p.closePerfReader()
	p.cleanup()

	if err := p.loadAndAttachBPF(); err != nil {
		log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
		p.cleanup()
		return err
	}

	if p.probeConfig[probe.ProbeTypeEvent] != nil {
		var err error
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspPlEvent, int(unsafe.Sizeof(bpfInspPlEventT{})))
		if err != nil {
			log.Errorf("%s error create perf reader, err: %v", probeName, err)
			return err
		}

		go p.perfLoop()
	}

	return nil
}

func (p *packetLossProbe) enableStack() bool {
	cfg := p.probeConfig[probe.ProbeTypeEvent]
	return cfg != nil && cfg.enableStack
}

func (p *packetLossProbe) loadAndAttachBPF() error {
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove limit failed: %s", err.Error())
	}

	opts := ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			KernelTypes: bpfutil.LoadBTFSpecOrNil(),
		},
	}
	if err := loadBpfObjects(&p.objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %s", err.Error())
	}

	if p.enableStack() {
		if err := bpfutil.UpdateFeatureSwitch(p.objs.InspPacketlossFeatureSwitch, featureSwitchEnablePacketLossStackKey, 1); err != nil {
			return fmt.Errorf("failed update packetloss feature switch: %w", err)
		}
	}

	pl, err := link.Tracepoint("skb", "kfree_skb", p.objs.KfreeSkb, &link.TracepointOptions{})
	if err != nil {
		return fmt.Errorf("link tracepoint kfree_skb failed: %w", err)
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

	return ok
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

		if p.enableStack() {
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
		}

		if p.sink != nil {
			p.sink <- evt
		}
	}
}
