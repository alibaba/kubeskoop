package tracetcpretrans

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"
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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_tcpretrans_event_t -type tuple -type addr bpf ../../../../bpf/tcpretrans.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	retransTotal = "total"
	retransFast  = "fast"

	TCPRetrans = "TCPRetrans"
)

var (
	probeName        = "tcpretrans"
	ignoreSymbolList = map[string]struct{}{}

	_tcpRetransProbe = &tcpRetransProbe{}
)

func init() {
	var err error
	_tcpRetransProbe.cache, err = lru.New[probe.Tuple, *Counter](102400)
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
			{Name: retransTotal, ValueType: prometheus.CounterValue},
			//{Name: retransFast, ValueType: prometheus.CounterValue},
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

type probeConfig struct {
}

type metricsProbe struct {
}

func (p *metricsProbe) Start(_ context.Context) error {
	return _tcpRetransProbe.start(probe.ProbeTypeMetrics, &probeConfig{})
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return _tcpRetransProbe.stop(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) collectOnce(emit probe.Emit) error {
	keys := _tcpRetransProbe.cache.Keys()
	for _, tuple := range keys {
		counter, ok := _tcpRetransProbe.cache.Get(tuple)
		if !ok || counter == nil {
			continue
		}

		labels := probe.BuildTupleMetricsLabels(&tuple)
		emit(retransTotal, labels, float64(counter.Total))
		//emit(retransFast, labels, float64(counter.Fast))
	}
	return nil
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(_ context.Context) error {
	err := _tcpRetransProbe.start(probe.ProbeTypeEvent, &probeConfig{})
	if err != nil {
		return err
	}

	_tcpRetransProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(_ context.Context) error {
	return _tcpRetransProbe.stop(probe.ProbeTypeEvent)
}

type Counter struct {
	Total uint32
	Fast  uint32
}

type tcpRetransProbe struct {
	objs        bpfObjects
	links       []link.Link
	sink        chan<- *probe.Event
	probeConfig [probe.ProbeTypeCount]*probeConfig
	lock        sync.Mutex
	perfReader  *perf.Reader

	cache *lru.Cache[probe.Tuple, *Counter]
}

func (p *tcpRetransProbe) probeCount() int {
	var ret int
	for _, cfg := range p.probeConfig {
		if cfg != nil {
			ret++
		}
	}
	return ret
}
func (p *tcpRetransProbe) stop(probeType probe.Type) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.probeConfig[probeType] == nil {
		return fmt.Errorf("probe %s never start", probeType)
	}

	p.probeConfig[probeType] = nil

	if p.probeCount() == 0 {
		p.cleanup()
	}

	return nil
}

func (p *tcpRetransProbe) cleanup() {
	if p.perfReader != nil {
		p.perfReader.Close()
		p.perfReader = nil
	}

	for _, link := range p.links {
		link.Close()
	}

	p.links = nil

	p.objs.Close()
}

func (p *tcpRetransProbe) start(probeType probe.Type, cfg *probeConfig) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.probeConfig[probeType] != nil {
		return fmt.Errorf("%s(%s) has already started", probeName, probeType)
	}

	p.probeConfig[probeType] = cfg

	if err := p.reinstallBPFLocked(); err != nil {
		return fmt.Errorf("%s failed install ebpf: %w", probeName, err)
	}

	return nil
}

func (p *tcpRetransProbe) reinstallBPFLocked() (err error) {
	p.cleanup()

	defer func() {
		if err != nil {
			p.cleanup()
		}
	}()

	if err = p.loadAndAttachBPF(); err != nil {
		return fmt.Errorf("%s failed load and attach bpf, err: %w", probeName, err)
	}

	p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspTcpRetransEvent, int(unsafe.Sizeof(bpfInspTcpretransEventT{})))
	if err != nil {
		return fmt.Errorf("%s error create perf reader, err: %w", probeName, err)
	}

	go p.perfLoop()

	return nil
}

func (p *tcpRetransProbe) loadAndAttachBPF() error {
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

	pl, err := link.Tracepoint("tcp", "tcp_retransmit_skb", p.objs.bpfPrograms.Tcpretrans, &link.TracepointOptions{})

	if err != nil {
		return fmt.Errorf("link raw tracepoint tcp_retransmit_skb failed: %s", err.Error())
	}
	p.links = append(p.links, pl)
	return nil
}

func toProbeTuple(t *bpfTuple) *probe.Tuple {
	t.L3Proto = syscall.ETH_P_IPV6
	return &probe.Tuple{
		Protocol: t.L4Proto,
		Src:      bpfutil.GetAddrStr(t.L3Proto, t.Saddr.V6addr),
		Dst:      bpfutil.GetAddrStr(t.L3Proto, t.Daddr.V6addr),
		Sport:    t.Sport,
		Dport:    t.Dport,
	}
}

func (p *tcpRetransProbe) perfLoop() {
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

		var event bpfInspTcpretransEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.NativeEndian, &event); err != nil {
			log.Errorf("%s failed parsing event, err: %v", probeName, err)
			continue
		}

		tuple := toProbeTuple(&event.Tuple)

		v, ok := p.cache.Get(*tuple)
		if !ok {
			v = &Counter{}
			p.cache.Add(*tuple, v)
		}
		v.Total++

		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Type:      TCPRetrans,
			Labels:    probe.BuildTupleEventLabels(tuple),
		}

		//TODO add trigger to enable/disable stack
		stacks, err := bpfutil.GetSymsByStack(uint32(event.StackId), p.objs.InspTcpRetransStack)
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
