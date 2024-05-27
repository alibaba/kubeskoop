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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target=${GOARCH} -cc clang -cflags $BPF_CFLAGS -type insp_kl_event_t  bpf ../../../../bpf/kernellatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_${GOARCH}

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

	RXKERNEL_SLOW_METRIC      = "rxslow"
	TXKERNEL_SLOW_METRIC      = "txslow"
	RXKERNEL_SLOW100MS_METRIC = "rxslow100ms"
	TXKERNEL_SLOW100MS_METRIC = "txslow100ms"

	probeTypeEvent   = 0
	probeTypeMetrics = 1
)

var (
	metrics      = []string{RXKERNEL_SLOW_METRIC, RXKERNEL_SLOW100MS_METRIC, TXKERNEL_SLOW_METRIC, TXKERNEL_SLOW100MS_METRIC}
	probeName    = "kernellatency"
	latencyProbe = &kernelLatencyProbe{
		metricsMap: make(map[string]map[uint32]uint64),
	}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}

func metricsProbeCreator() (probe.MetricsProbe, error) {
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

func (p *metricsProbe) Start(ctx context.Context) error {
	return latencyProbe.start(ctx, probe.ProbeTypeMetrics)
}

func (p *metricsProbe) Stop(ctx context.Context) error {
	return latencyProbe.stop(ctx, probe.ProbeTypeMetrics)
}

func (p *metricsProbe) CollectOnce() (map[string]map[uint32]uint64, error) {
	return latencyProbe.copyMetricsMap(), nil
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(ctx context.Context) error {
	err := latencyProbe.start(ctx, probe.ProbeTypeEvent)
	if err != nil {
		return err
	}

	latencyProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(ctx context.Context) error {
	return latencyProbe.stop(ctx, probe.ProbeTypeEvent)
}

type kernelLatencyProbe struct {
	objs        bpfObjects
	links       []link.Link
	sink        chan<- *probe.Event
	refcnt      [2]int
	lock        sync.Mutex
	perfReader  *perf.Reader
	metricsMap  map[string]map[uint32]uint64
	metricsLock sync.RWMutex
}

func (p *kernelLatencyProbe) stopLocked(probeType probe.Type) error {
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

func (p *kernelLatencyProbe) stop(_ context.Context, probeType probe.Type) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.stopLocked(probeType)
}

func (p *kernelLatencyProbe) cleanup() error {
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

func (p *kernelLatencyProbe) copyMetricsMap() map[string]map[uint32]uint64 {
	p.metricsLock.RLock()
	defer p.metricsLock.RUnlock()
	return probe.CopyLegacyMetricsMap(p.metricsMap)
}

func (p *kernelLatencyProbe) totalReferenceCountLocked() int {
	var c int
	for _, n := range p.refcnt {
		c += n
	}
	return c
}

func (p *kernelLatencyProbe) start(_ context.Context, probeType probe.Type) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.refcnt[probeType] != 0 {
		return fmt.Errorf("%s(%s) has already started", probeName, probeType)
	}

	p.refcnt[probeType]++
	if p.totalReferenceCountLocked() == 1 {
		if err = p.loadAndAttachBPF(); err != nil {
			p.refcnt[probeType]--
			log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
			_ = p.cleanup()
			return fmt.Errorf("%s failed load bpf: %w", probeName, err)
		}
	}

	if p.refcnt[probe.ProbeTypeEvent] == 1 {
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspKlatencyEvent, int(unsafe.Sizeof(bpfInspKlEventT{})))
		if err != nil {
			log.Errorf("%s failed create perf reader, err: %v", probeName, err)
			_ = p.stopLocked(probeType)
			return fmt.Errorf("%s failed create bpf reader: %w", probeName, err)
		}

		go p.perfLoop()
	}

	return nil
}

func (p *kernelLatencyProbe) updateMetrics(netns uint32, metrics string) {
	p.metricsLock.Lock()
	defer p.metricsLock.Unlock()
	if _, ok := p.metricsMap[metrics]; !ok {
		p.metricsMap[metrics] = make(map[uint32]uint64)
	}

	p.metricsMap[metrics][netns]++
}

func (p *kernelLatencyProbe) perfLoop() {
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

		var event bpfInspKlEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Errorf("%s failed parsing event, err: %v", probeName, err)
			continue
		}

		netns := event.SkbMeta.Netns
		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Labels:    probe.LegacyEventLabels(netns),
		}
		/*
		   #define RX_KLATENCY 1
		   #define TX_KLATENCY 2
		*/
		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		switch event.Direction {
		case 1:
			evt.Type = RXKERNEL_SLOW
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
			evt.Message = fmt.Sprintf("%s %s", tuple, strings.Join(latency, " "))
			p.updateMetrics(netns, RXKERNEL_SLOW_METRIC)
		case 2:
			evt.Type = TXKERNEL_SLOW
			latency := []string{fmt.Sprintf("latency=%s", bpfutil.GetHumanTimes(event.Latency))}
			if event.Point3 > event.Point1 && event.Point1 != 0 {
				latency = append(latency, fmt.Sprintf("LOCAL_OUT=%s", bpfutil.GetHumanTimes(event.Point3-event.Point1)))
			}
			if event.Point4 > event.Point3 && event.Point3 != 0 {
				latency = append(latency, fmt.Sprintf("POSTROUTING=%s", bpfutil.GetHumanTimes(event.Point4-event.Point3)))
			}
			evt.Message = fmt.Sprintf("%s %s", tuple, strings.Join(latency, " "))
			p.updateMetrics(netns, TXKERNEL_SLOW_METRIC)
		default:
			log.Infof("%s failed parsing event %s, ignore", probeName, util.ToJSONString(evt))
			continue
		}

		if p.sink != nil {
			log.Debugf("%s sink event %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}
}

func (p *kernelLatencyProbe) loadAndAttachBPF() error {
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

	progrcv, err := link.Kprobe(HOOK_IPRCV, p.objs.KlatencyIpRcv, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPRCV: %s", err.Error())
	}
	p.links = append(p.links, progrcv)

	progrcvfin, err := link.Kprobe(HOOK_IPRCVFIN, p.objs.KlatencyIpRcvFinish, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPRCVFIN: %s", err.Error())
	}
	p.links = append(p.links, progrcvfin)

	proglocal, err := link.Kprobe(HOOK_IPLOCAL, p.objs.KlatencyIpLocalDeliver, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPRCV: %s", err.Error())
	}
	p.links = append(p.links, proglocal)

	proglocalfin, err := link.Kprobe(HOOK_IPLOCALFIN, p.objs.KlatencyIpLocalDeliverFinish, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPLOCALFIN: %s", err.Error())
	}
	p.links = append(p.links, proglocalfin)

	progxmit, err := link.Kprobe(HOOK_IPXMIT, p.objs.KlatencyIpQueueXmit, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPXMIT: %s", err.Error())
	}
	p.links = append(p.links, progxmit)

	proglocalout, err := link.Kprobe(HOOK_IPLOCALOUT, p.objs.KlatencyIpLocal, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPLOCALOUT: %s", err.Error())
	}
	p.links = append(p.links, proglocalout)

	progoutput, err := link.Kprobe(HOOK_IPOUTPUT, p.objs.KlatencyIpOutput, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPOUTPUT: %s", err.Error())
	}
	p.links = append(p.links, progoutput)

	progfin, err := link.Kprobe(HOOK_IPOUTPUTFIN, p.objs.KlatencyIpFinishOutput2, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link HOOK_IPOUTPUTFIN: %s", err.Error())
	}
	p.links = append(p.links, progfin)

	progkfree, err := link.Kprobe("kfree_skb", p.objs.ReportKfree, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link kfree_skb: %s", err.Error())
	}
	p.links = append(p.links, progkfree)

	progconsume, err := link.Kprobe("consume_skb", p.objs.ReportConsume, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link consume_skb: %s", err.Error())
	}
	p.links = append(p.links, progconsume)

	return nil
}
