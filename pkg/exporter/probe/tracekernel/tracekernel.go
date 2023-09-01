package tracekernel

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
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/exp/slog"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_kl_event_t  bpf ../../../../bpf/kernellatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	TXKERNEL_SLOW = "TXKERNEL_SLOW"
	RXKERNEL_SLOW = "RXKERNEL_SLOW"

	HOOK_IPRCV       = "ip_rcv"
	HOOK_IPRCVFIN    = "ip_rcv_finish"
	HOOK_IPLOCAL     = "ip_local_deliver"
	HOOK_IPLOCALFIN  = "ip_local_deliver_finish"
	HOOK_IPXMIT      = "__ip_queue_xmit"
	HOOK_IPLOCALOUT  = "ip_local_out"
	HOOK_IPOUTPUT    = "ip_output"
	HOOK_IPOUTPUTFIN = "ip_finish_output2"

	RXKERNEL_SLOW_METRIC      = "kernellatency_rxslow"
	TXKERNEL_SLOW_METRIC      = "kernellatency_txslow"
	RXKERNEL_SLOW100MS_METRIC = "kernellatency_rxslow100ms"
	TXKERNEL_SLOW100MS_METRIC = "kernellatency_txslow100ms"
)

var (
	ModuleName = "insp_kernellatency" // nolint
	probe      = &KernelLatencyProbe{once: sync.Once{}, mtx: sync.Mutex{}, enabledProbes: map[proto.ProbeType]bool{}}
	objs       = bpfObjects{}
	links      = []link.Link{}

	events     = []string{RXKERNEL_SLOW, TXKERNEL_SLOW}
	metrics    = []string{RXKERNEL_SLOW_METRIC, RXKERNEL_SLOW100MS_METRIC, TXKERNEL_SLOW_METRIC, TXKERNEL_SLOW100MS_METRIC}
	metricsMap = map[string]map[uint32]uint64{}
)

func GetProbe() *KernelLatencyProbe {
	return probe
}

func init() {
	for m := range metrics {
		metricsMap[metrics[m]] = map[uint32]uint64{}
	}
}

type KernelLatencyProbe struct {
	enable        bool
	sub           chan<- proto.RawEvent
	once          sync.Once
	mtx           sync.Mutex
	enabledProbes map[proto.ProbeType]bool
}

func (p *KernelLatencyProbe) Name() string {
	return ModuleName
}

func (p *KernelLatencyProbe) Ready() bool {
	return p.enable
}

func (p *KernelLatencyProbe) GetEventNames() []string {
	return events
}

func (p *KernelLatencyProbe) Close(probeType proto.ProbeType) error {
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
	metricsMap = map[string]map[uint32]uint64{}

	delete(p.enabledProbes, probeType)
	return nil
}

func (p *KernelLatencyProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *KernelLatencyProbe) GetMetricNames() []string {
	return metrics
}

func (p *KernelLatencyProbe) Collect(_ context.Context) (map[string]map[uint32]uint64, error) {
	return metricsMap, nil
}

func (p *KernelLatencyProbe) Start(ctx context.Context, probeType proto.ProbeType) {
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

	go p.startRX(ctx)
	// go p.startTX(ctx)
}

func (p *KernelLatencyProbe) updateMetrics(netns uint32, metric string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if _, ok := metricsMap[metric]; ok {
		metricsMap[metric][netns]++
	}
}

func (p *KernelLatencyProbe) startRX(ctx context.Context) {
	reader, err := perf.NewReader(objs.bpfMaps.InspKlatencyEvent, int(unsafe.Sizeof(bpfInspKlEventT{})))
	if err != nil {
		slog.Ctx(ctx).Warn("start new perf reader", "module", ModuleName, "err", err)
		return
	}

	for {
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

		var event bpfInspKlEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "err", err)
			continue
		}
		rawevt := proto.RawEvent{
			Netns: event.SkbMeta.Netns,
		}
		/*
		   #define RX_KLATENCY 1
		   #define TX_KLATENCY 2
		*/
		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		switch event.Direction {
		case 1:
			rawevt.EventType = RXKERNEL_SLOW
			latency := []string{fmt.Sprintf("latency:%s", bpfutil.GetHumanTimes(event.Latency))}
			if event.Point2 > event.Point1 {
				latency = append(latency, fmt.Sprintf("PREROUTING:%s", bpfutil.GetHumanTimes(event.Point2-event.Point1)))
			}
			if event.Point3 > event.Point2 && event.Point2 != 0 {
				latency = append(latency, fmt.Sprintf("ROUTE:%s", bpfutil.GetHumanTimes(event.Point3-event.Point2)))
			}
			if event.Point4 > event.Point3 && event.Point3 != 0 {
				latency = append(latency, fmt.Sprintf("LOCAL_IN:%s", bpfutil.GetHumanTimes(event.Point4-event.Point3)))
			}
			rawevt.EventBody = fmt.Sprintf("%s %s", tuple, strings.Join(latency, " "))
			p.updateMetrics(rawevt.Netns, RXKERNEL_SLOW_METRIC)
		case 2:
			rawevt.EventType = TXKERNEL_SLOW
			latency := []string{fmt.Sprintf("latency=%s", bpfutil.GetHumanTimes(event.Latency))}
			if event.Point3 > event.Point1 && event.Point1 != 0 {
				latency = append(latency, fmt.Sprintf("LOCAL_OUT=%s", bpfutil.GetHumanTimes(event.Point3-event.Point1)))
			}
			if event.Point4 > event.Point3 && event.Point3 != 0 {
				latency = append(latency, fmt.Sprintf("POSTROUTING=%s", bpfutil.GetHumanTimes(event.Point4-event.Point3)))
			}
			rawevt.EventBody = fmt.Sprintf("%s %s", tuple, strings.Join(latency, " "))
			p.updateMetrics(rawevt.Netns, TXKERNEL_SLOW_METRIC)
		default:
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "ignore", event)
			continue
		}

		if p.sub != nil {
			slog.Ctx(ctx).Debug("broadcast event", "module", ModuleName)
			p.sub <- rawevt
		}
	}
}

func loadSync() error {
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
	if err := loadBpfObjects(&objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %v", err)
	}

	progrcv, err := link.Kprobe(HOOK_IPRCV, objs.KlatencyIpRcv, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPRCV: %s", err.Error())
	}
	links = append(links, progrcv)

	progrcvfin, err := link.Kprobe(HOOK_IPRCVFIN, objs.KlatencyIpRcvFinish, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPRCVFIN: %s", err.Error())
	}
	links = append(links, progrcvfin)

	proglocal, err := link.Kprobe(HOOK_IPLOCAL, objs.KlatencyIpLocalDeliver, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPRCV: %s", err.Error())
	}
	links = append(links, proglocal)

	proglocalfin, err := link.Kprobe(HOOK_IPLOCALFIN, objs.KlatencyIpLocalDeliverFinish, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPLOCALFIN: %s", err.Error())
	}
	links = append(links, proglocalfin)

	progxmit, err := link.Kprobe(HOOK_IPXMIT, objs.KlatencyIpQueueXmit, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPXMIT: %s", err.Error())
	}
	links = append(links, progxmit)

	proglocalout, err := link.Kprobe(HOOK_IPLOCALOUT, objs.KlatencyIpLocal, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPLOCALOUT: %s", err.Error())
	}
	links = append(links, proglocalout)

	progoutput, err := link.Kprobe(HOOK_IPOUTPUT, objs.KlatencyIpOutput, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPOUTPUT: %s", err.Error())
	}
	links = append(links, progoutput)

	progfin, err := link.Kprobe(HOOK_IPOUTPUTFIN, objs.KlatencyIpFinishOutput2, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPOUTPUTFIN: %s", err.Error())
	}
	links = append(links, progfin)

	progkfree, err := link.Kprobe("kfree_skb", objs.ReportKfree, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link kfree_skb: %s", err.Error())
	}
	links = append(links, progkfree)

	progconsume, err := link.Kprobe("consume_skb", objs.ReportConsume, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link consume_skb: %s", err.Error())
	}
	links = append(links, progconsume)

	return nil
}
