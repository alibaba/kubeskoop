package tracepacketloss

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/util"
	log "github.com/sirupsen/logrus"

	"math/bits"
	"strings"
	"sync"
	"unsafe"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_pl_event_t -type insp_pl_metric_t bpf ../../../../bpf/packetloss.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	PACKETLOSS_ABNORMAL  = "abnormal"
	PACKETLOSS_TOTAL     = "total"
	PACKETLOSS_NETFILTER = "netfilter"
	PACKETLOSS_TCPSTATEM = "tcpstatm"
	PACKETLOSS_TCPRCV    = "tcprcv"
	PACKETLOSS_TCPHANDLE = "tcphandle"

	PACKETLOSS = "PACKETLOSS"
)

var (
	ignoreSymbolList  = map[string]struct{}{}
	uselessSymbolList = map[string]struct{}{}

	netfilterSymbol = "nf_hook_slow"
	tcpstatmSymbol  = "tcp_rcv_state_process"
	tcprcvSymbol    = "tcp_v4_rcv"
	tcpdorcvSymbol  = "tcp_v4_do_rcv"

	probeName = "packetloss"

	packetLossMetrics = []string{PACKETLOSS_TCPHANDLE, PACKETLOSS_TCPRCV, PACKETLOSS_ABNORMAL, PACKETLOSS_TOTAL, PACKETLOSS_NETFILTER, PACKETLOSS_TCPSTATEM}

	_packetLossProbe = &packetLossProbe{}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}

func metricsProbeCreator(_ map[string]interface{}) (probe.MetricsProbe, error) {
	p := &metricsProbe{}
	batchMetrics := probe.NewLegacyBatchMetricsWithUnderscore(probeName, packetLossMetrics, p.CollectOnce)

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

func (p *metricsProbe) CollectOnce() (map[string]map[uint32]uint64, error) {
	return _packetLossProbe.collect()
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

type packetLossProbe struct {
	objs       bpfObjects
	links      []link.Link
	sink       chan<- *probe.Event
	refcnt     [probe.ProbeTypeCount]int
	lock       sync.Mutex
	perfReader *perf.Reader
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

		if p.refcnt[probe.ProbeTypeEvent] == 1 {
			p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspPlEvent, int(unsafe.Sizeof(bpfInspPlEventT{})))
			if err != nil {
				log.Errorf("%s error create perf reader, err: %v", probeName, err)
				_ = p.stop(ctx, probeType)
				return
			}

			go p.perfLoop()
		}
	}
	return nil
}

func (p *packetLossProbe) collect() (map[string]map[uint32]uint64, error) {
	//TODO metrics of packetloss should be counter, not gauge.
	// we should create metrics from events
	resMap := make(map[string]map[uint32]uint64)
	for _, metric := range packetLossMetrics {
		resMap[metric] = make(map[uint32]uint64)
	}

	m := p.objs.bpfMaps.InspPlMetric
	if m == nil {
		log.Warnf("%s get metric map with nil", probeName)
		return nil, nil
	}
	var (
		value   uint64
		entries = m.Iterate()
		key     = bpfInspPlMetricT{}
	)

	// if no entity found, do not report metric
	ets := nettop.GetAllEntity()
	if len(ets) == 0 {
		return resMap, nil
	}

	for _, et := range ets {
		// default set all stat to zero to prevent empty metric
		for _, metric := range packetLossMetrics {
			resMap[metric][uint32(et.GetNetns())] = 0
		}
	}

	for entries.Next(&key, &value) {
		// in tcp_v4_do_rcv situation, after sock_rps_save_rxhash, skb->dev was removed and sock was not bind
		if key.Netns == 0 {
			continue
		}

		// if _, ok := res[PACKETLOSS_TOTAL][key.Netns]; ok {
		// 	res[PACKETLOSS_TOTAL][key.Netns] += value
		// } else {
		// 	res[PACKETLOSS_TOTAL][key.Netns] += value
		// }

		sym, err := bpfutil.GetSymPtFromBpfLocation(key.Location)
		if err != nil {
			log.Warnf("%s get sym failed, location: %x, err: %v", probeName, key.Location, err)
			continue
		}

		switch sym.GetName() {
		case netfilterSymbol:
			resMap[PACKETLOSS_NETFILTER][key.Netns] += value
		case tcpstatmSymbol:
			resMap[PACKETLOSS_TCPSTATEM][key.Netns] += value
		case tcprcvSymbol:
			resMap[PACKETLOSS_TCPRCV][key.Netns] += value
		case tcpdorcvSymbol:
			resMap[PACKETLOSS_TCPHANDLE][key.Netns] += value
		default:
			if _, ok := ignoreSymbolList[sym.GetName()]; !ok {
				resMap[PACKETLOSS_ABNORMAL][key.Netns] += value
			}
			resMap[PACKETLOSS_TOTAL][key.Netns] += value
		}

		// if _, ok := ignoreSymbolList[sym.GetName()]; !ok {
		// 	if _, ok := res[PACKETLOSS_ABNORMAL][key.Netns]; ok {
		// 		res[PACKETLOSS_ABNORMAL][key.Netns] += value
		// 	} else {
		// 		res[PACKETLOSS_ABNORMAL][key.Netns] += value
		// 	}
		// }
	}

	return resMap, nil
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

func (p *packetLossProbe) perfLoop() {
	for {
	anothor_loop:
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
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Errorf("%s failed parsing event, err: %v", probeName, err)
			continue
		}
		// filter netlink/unixsock/tproxy packet
		if event.Tuple.Dport == 0 && event.Tuple.Sport == 0 {
			continue
		}

		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Type:      PACKETLOSS,
			Labels:    probe.LagacyEventLabels(event.SkbMeta.Netns),
		}

		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))

		stacks, err := bpfutil.GetSymsByStack(uint32(event.StackId), p.objs.InspPlStack)
		if err != nil {
			log.Warnf("%s failed get sym by stack, err: %v", probeName, err)
			continue
		}
		strs := []string{}
		for _, sym := range stacks {
			if _, ok := ignoreSymbolList[sym.GetName()]; ok {
				goto anothor_loop
			}
			if _, ok := uselessSymbolList[sym.GetName()]; ok {
				continue
			}
			strs = append(strs, sym.GetExpr())
		}

		stackStr := strings.Join(strs, " ")

		evt.Message = fmt.Sprintf("%s stacktrace:%s", tuple, stackStr)

		if p.sink != nil {
			log.Debugf("%s sink event %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}
}
