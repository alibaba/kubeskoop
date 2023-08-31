package tracepacketloss

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"math/bits"
	"strings"
	"sync"
	"unsafe"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/exp/slog"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_pl_event_t -type insp_pl_metric_t bpf ../../../../bpf/packetloss.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	ModuleName           = "insp_packetloss"
	PACKETLOSS_ABNORMAL  = "packetloss_abnormal"
	PACKETLOSS_TOTAL     = "packetloss_total"
	PACKETLOSS_NETFILTER = "packetloss_netfilter"
	PACKETLOSS_TCPSTATEM = "packetloss_tcpstatm"
	PACKETLOSS_TCPRCV    = "packetloss_tcprcv"
	PACKETLOSS_TCPHANDLE = "packetloss_tcphandle"

	PACKETLOSS = "PACKETLOSS"
)

var (
	ignoreSymbolList  = map[string]struct{}{}
	uselessSymbolList = map[string]struct{}{}

	netfilterSymbol = "nf_hook_slow"
	tcpstatmSymbol  = "tcp_rcv_state_process"
	tcprcvSymbol    = "tcp_v4_rcv"
	tcpdorcvSymbol  = "tcp_v4_do_rcv"

	probe  = &PacketLossProbe{enabledProbes: map[proto.ProbeType]bool{}}
	objs   = bpfObjects{}
	links  = []link.Link{}
	events = []string{PACKETLOSS}

	packetLossMetrics = []string{PACKETLOSS_TCPHANDLE, PACKETLOSS_TCPRCV, PACKETLOSS_ABNORMAL, PACKETLOSS_TOTAL, PACKETLOSS_NETFILTER, PACKETLOSS_TCPSTATEM}
)

func init() {
	// sk_stream_kill_queues: skb moved to sock rqueue and then will be cleanup by this symbol
	ignore("sk_stream_kill_queues")
	// tcp_v4_rcv: skb of ingress stream will pass the symbol.
	// ignore("tcp_v4_rcv")
	// ignore("tcp_v6_rcv")
	// // tcp_v4_do_rcv: skb drop when check CSUMERRORS
	// ignore("tcp_v4_do_rcv")
	// skb_queue_purge netlink recv function
	ignore("skb_queue_purge")
	ignore("nfnetlink_rcv_batch")
	// unix_stream_connect unix stream io is not cared
	ignore("unix_stream_connect")
	ignore("skb_release_data")

	// kfree_skb_list: free skb batch
	useless("kfree_skb_list")
	// kfree_skb_reason: wrapper of kfree_skb in newer kernel
	useless("kfree_skb_reason")
	// kfree_skb.part
	useless("kfree_skb.part.0")
	// kfree_skb
	useless("kfree_skb")
}

func ignore(sym string) {
	ignoreSymbolList[sym] = struct{}{}
}

func useless(sym string) {
	uselessSymbolList[sym] = struct{}{}
}

func GetProbe() *PacketLossProbe {
	return probe
}

type PacketLossProbe struct {
	enable        bool
	sub           chan<- proto.RawEvent
	once          sync.Once
	mtx           sync.Mutex
	enabledProbes map[proto.ProbeType]bool
}

func (p *PacketLossProbe) Name() string {
	return ModuleName
}

func (p *PacketLossProbe) Ready() bool {
	return p.enable
}

func (p *PacketLossProbe) GetEventNames() []string {
	return events
}

func (p *PacketLossProbe) GetMetricNames() []string {
	res := []string{}
	for _, m := range packetLossMetrics {
		res = append(res, strings.ToLower(m))
	}
	return res
}

func (p *PacketLossProbe) Close(probeType proto.ProbeType) error {
	if !p.enable {
		return nil
	}

	if _, ok := p.enabledProbes[probeType]; !ok {
		return nil
	}
	if len(p.enabledProbes) > 1 {
		delete(p.enabledProbes, probeType)
		return nil
	}

	for _, link := range links {
		link.Close()
	}
	links = []link.Link{}
	p.enable = false
	p.once = sync.Once{}

	delete(p.enabledProbes, probeType)
	return nil
}

func (p *PacketLossProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *PacketLossProbe) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	resMap := make(map[string]map[uint32]uint64)
	for _, metric := range packetLossMetrics {
		resMap[metric] = make(map[uint32]uint64)
	}

	m := objs.bpfMaps.InspPlMetric
	if m == nil {
		slog.Ctx(ctx).Warn("get metric map with nil", "module", ModuleName)
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
			slog.Ctx(ctx).Warn("get sym failed", "err", err, "module", ModuleName, "location", key.Location)
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

func (p *PacketLossProbe) Start(ctx context.Context, probeType proto.ProbeType) {
	if p.enable {
		p.enabledProbes[probeType] = true
		return
	}

	p.once.Do(func() {
		err := loadSync()
		if err != nil {
			slog.Ctx(ctx).Warn("start", "module", ModuleName, "err", err)
			return
		}
		p.enable = true
	})

	if !p.enable {
		// if load failed, do not start process
		return
	}
	p.enabledProbes[probeType] = true

	reader, err := perf.NewReader(objs.bpfMaps.InspPlEvent, int(unsafe.Sizeof(bpfInspPlEventT{})))
	if err != nil {
		slog.Ctx(ctx).Warn("start new perf reader", "module", ModuleName, "err", err)
		return
	}

	for {
	anothor_loop:
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				slog.Ctx(ctx).Info("received signal, exiting..", "module", ModuleName)
				return
			}
			slog.Ctx(ctx).Info("reading from reader", "module", ModuleName, "err", err)
			continue
		}

		if record.LostSamples != 0 {
			slog.Ctx(ctx).Info("Perf event ring buffer full", "module", ModuleName, "drop samples", record.LostSamples)
			continue
		}

		var event bpfInspPlEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "err", err)
			continue
		}
		// filter netlink/unixsock/tproxy packet
		if event.Tuple.Dport == 0 && event.Tuple.Sport == 0 {
			continue
		}
		rawevt := proto.RawEvent{
			Netns:     event.SkbMeta.Netns,
			EventType: PACKETLOSS,
		}

		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))

		stacks, err := bpfutil.GetSymsByStack(uint32(event.StackId), objs.InspPlStack)
		if err != nil {
			slog.Ctx(ctx).Warn("get sym by stack with", "module", ModuleName, "err", err)
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

		rawevt.EventBody = fmt.Sprintf("%s stacktrace:%s", tuple, stackStr)
		if p.sub != nil {
			slog.Ctx(ctx).Debug("broadcast event", "module", ModuleName)
			p.sub <- rawevt
		}
	}
}

func loadSync() error {
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove limit failed: %s", err.Error())
	}

	opts := ebpf.CollectionOptions{}

	opts.Programs = ebpf.ProgramOptions{
		KernelTypes: bpfutil.LoadBTFSpecOrNil(),
	}

	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %s", err.Error())
	}

	pl, err := link.Tracepoint("skb", "kfree_skb", objs.KfreeSkb, &link.TracepointOptions{})
	if err != nil {
		return fmt.Errorf("link tracepoint kfree_skb failed: %s", err.Error())
	}
	links = append(links, pl)
	return nil
}
