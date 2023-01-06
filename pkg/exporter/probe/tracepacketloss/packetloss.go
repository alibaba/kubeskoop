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

	bpfutil2 "github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/exp/slog"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_pl_event_t -type insp_pl_metric_t bpf ../../../bpf/packetloss.c -- -I../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	MODULE_NAME         = "insp_packetloss"
	PACKETLOSS_ABNORMAL = "packetloss_abnormal"
	PACKETLOSS_TOTAL    = "packetloss_total"

	PACKETLOSS = "PACKETLOSS"
)

var (
	ignoreSymbolList  = map[string]struct{}{}
	uselessSymbolList = map[string]struct{}{}

	probe  = &PacketLossProbe{}
	objs   = bpfObjects{}
	links  = []link.Link{}
	events = []string{PACKETLOSS}

	packetLossMetrics = []string{PACKETLOSS_ABNORMAL, PACKETLOSS_TOTAL}
)

func init() {
	// sk_stream_kill_queues: skb moved to sock rqueue and then will be cleanup by this symbol
	ignore("sk_stream_kill_queues")
	// tcp_v4_rcv: skb of ingress stream will pass the symbol.
	ignore("tcp_v4_rcv")
	ignore("tcp_v6_rcv")
	// tcp_v4_do_rcv: skb drop when check CSUMERRORS
	ignore("tcp_v4_do_rcv")
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
	enable bool
	sub    chan<- proto.RawEvent
	once   sync.Once
	mtx    sync.Mutex
}

func (p *PacketLossProbe) Name() string {
	return MODULE_NAME
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

func (p *PacketLossProbe) Close() error {
	if p.enable {
		for _, link := range links {
			link.Close()
		}
		links = []link.Link{}
	}

	return nil
}

func (p *PacketLossProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *PacketLossProbe) Collect(ctx context.Context) (map[string]map[uint32]uint64, error) {
	res := map[string]map[uint32]uint64{
		PACKETLOSS_ABNORMAL: {},
		PACKETLOSS_TOTAL:    {},
	}
	m := objs.bpfMaps.InspPlMetric
	if m == nil {
		slog.Ctx(ctx).Warn("get metric map with nil", "module", MODULE_NAME)
		return nil, nil
	}
	var (
		value   uint64
		entries = m.Iterate()
		key     = bpfInspPlMetricT{}
	)

	for entries.Next(&key, &value) {
		// in tcp_v4_do_rcv situation, after sock_rps_save_rxhash, skb->dev was removed and sock was not bind
		if key.Netns == 0 {
			continue
		}

		if _, ok := res[PACKETLOSS_TOTAL][key.Netns]; ok {
			res[PACKETLOSS_TOTAL][key.Netns] += value
		} else {
			res[PACKETLOSS_TOTAL][key.Netns] += value
		}

		sym, err := bpfutil2.GetSymPtFromBpfLocation(key.Location)
		if err != nil {
			slog.Ctx(ctx).Warn("get sym failed", "err", err, "module", MODULE_NAME, "location", key.Location)
			continue
		}

		if _, ok := ignoreSymbolList[sym.GetName()]; !ok {
			if _, ok := res[PACKETLOSS_ABNORMAL][key.Netns]; ok {
				res[PACKETLOSS_ABNORMAL][key.Netns] += value
			} else {
				res[PACKETLOSS_ABNORMAL][key.Netns] += value
			}
		}
	}

	return res, nil
}

func (p *PacketLossProbe) Start(ctx context.Context) {
	if p.enable {
		return
	}

	p.once.Do(func() {
		err := loadSync()
		if err != nil {
			slog.Ctx(ctx).Warn("start", "module", MODULE_NAME, "err", err)
			return
		}
		p.enable = true
	})

	reader, err := perf.NewReader(objs.bpfMaps.InspPlEvent, int(unsafe.Sizeof(bpfInspPlEventT{})))
	if err != nil {
		slog.Ctx(ctx).Warn("start new perf reader", "module", MODULE_NAME, "err", err)
		return
	}

	for {
	anothor_loop:
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				slog.Ctx(ctx).Info("received signal, exiting..", "module", MODULE_NAME)
				return
			}
			slog.Ctx(ctx).Info("reading from reader", "module", MODULE_NAME, "err", err)
			continue
		}

		if record.LostSamples != 0 {
			slog.Ctx(ctx).Info("Perf event ring buffer full", "module", MODULE_NAME, "drop samples", record.LostSamples)
			continue
		}

		var event bpfInspPlEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", MODULE_NAME, "err", err)
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

		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil2.GetProtoStr(event.Tuple.L4Proto), bpfutil2.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil2.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))

		stacks, err := bpfutil2.GetSymsByStack(uint32(event.StackId), objs.InspPlStack)
		if err != nil {
			slog.Ctx(ctx).Warn("get sym by stack with", "module", MODULE_NAME, "err", err)
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
			slog.Ctx(ctx).Debug("broadcast event", "module", MODULE_NAME)
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
		KernelTypes: bpfutil2.LoadBTFSpecOrNil(),
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
