package tracenetif

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
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

// nolint
//
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -target=${GOARCH} -cc clang -cflags $BPF_CFLAGS -type insp_nftxlat_event_t -type insp_nftxlat_metric_t bpf ../../../../bpf/netiftxlatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_${GOARCH}
const (
	TXLAT_QDISC_SLOW  = "qdiscslow100ms"
	TXLAT_NETDEV_SLOW = "netdevslow100ms"
)

var (
	metrics              = []string{TXLAT_QDISC_SLOW, TXLAT_NETDEV_SLOW}
	probeName            = "netiftxlat"
	_netifTxlatencyProbe = &netifTxlatencyProbe{
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

func (p *metricsProbe) Start(_ context.Context) error {
	return _netifTxlatencyProbe.start(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return _netifTxlatencyProbe.stop(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) CollectOnce() (map[string]map[uint32]uint64, error) {
	return _netifTxlatencyProbe.copyMetricsMap(), nil
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(_ context.Context) error {
	err := _netifTxlatencyProbe.start(probe.ProbeTypeEvent)
	if err != nil {
		return err
	}

	_netifTxlatencyProbe.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(_ context.Context) error {
	return _netifTxlatencyProbe.stop(probe.ProbeTypeEvent)
}

type netifTxlatencyProbe struct {
	objs        bpfObjects
	links       []link.Link
	sink        chan<- *probe.Event
	refcnt      [probe.ProbeTypeCount]int
	lock        sync.Mutex
	perfReader  *perf.Reader
	metricsMap  map[string]map[uint32]uint64
	metricsLock sync.RWMutex
}

func (p *netifTxlatencyProbe) stop(probeType probe.Type) error {
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

func (p *netifTxlatencyProbe) cleanup() error {
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

func (p *netifTxlatencyProbe) copyMetricsMap() map[string]map[uint32]uint64 {
	p.metricsLock.RLock()
	defer p.metricsLock.RUnlock()
	return probe.CopyLegacyMetricsMap(p.metricsMap)
}

func (p *netifTxlatencyProbe) totalReferenceCountLocked() int {
	var c int
	for _, n := range p.refcnt {
		c += n
	}
	return c
}

func (p *netifTxlatencyProbe) start(probeType probe.Type) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.refcnt[probeType]++
	if p.totalReferenceCountLocked() == 1 {
		if err = p.loadAndAttachBPF(); err != nil {
			log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
			_ = p.cleanup()
			return fmt.Errorf("%s failed load bpf: %w", probeName, err)
		}

		// 初始化map的读接口
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspSklatEvent, int(unsafe.Sizeof(bpfInspNftxlatEventT{})))
		if err != nil {
			log.Errorf("%s failed create perf reader, err: %v", probeName, err)
			return err
		}

		go p.perfLoop()
	}

	return nil
}

func (p *netifTxlatencyProbe) updateMetrics(netns uint32, metrics string) {
	p.metricsLock.Lock()
	defer p.metricsLock.Unlock()
	if _, ok := p.metricsMap[metrics]; !ok {
		p.metricsMap[metrics] = make(map[uint32]uint64)
	}

	p.metricsMap[metrics][netns]++
}

func (p *netifTxlatencyProbe) perfLoop() {
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

		// 解析perf事件信息，输出为proto.RawEvent
		var event bpfInspNftxlatEventT
		// Parse the ringbuf event entry into a bpfEvent structure.
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Errorf("%s failed parsing event, err: %v", probeName, err)
			continue
		}

		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Labels:    probe.LagacyEventLabels(event.SkbMeta.Netns),
		}
		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		evt.Message = fmt.Sprintf("%s latency:%s", tuple, bpfutil.GetHumanTimes(event.Latency))
		/*#define THRESH
		#define ACTION_QDISC	    1
		#define ACTION_XMIT	        2
		*/
		if event.Type == 1 {
			evt.Type = "NETIFTXLAT_QDISC"
			p.updateMetrics(event.SkbMeta.Netns, TXLAT_QDISC_SLOW)
		} else if event.Type == 2 {
			evt.Type = "NETIFTXLAT_XMIT"
			p.updateMetrics(event.SkbMeta.Netns, TXLAT_NETDEV_SLOW)
		}

		// 分发给注册的dispatcher，其余逻辑由框架完成
		if p.sink != nil {
			log.Debugf("%s sink event %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}

}

func (p *netifTxlatencyProbe) Collect(_ context.Context) (map[string]map[uint32]uint64, error) {
	p.metricsLock.RLock()
	defer p.metricsLock.RUnlock()
	return probe.CopyLegacyMetricsMap(p.metricsMap), nil
}

func (p *netifTxlatencyProbe) loadAndAttachBPF() error {
	// 准备动作
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal(err)
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

	// 执行link操作，保存rawfd
	progQueue, err := link.Tracepoint("net", "net_dev_queue", p.objs.NetDevQueue, &link.TracepointOptions{})
	if err != nil {
		return err
	}
	p.links = append(p.links, progQueue)

	progStart, err := link.Tracepoint("net", "net_dev_start_xmit", p.objs.NetDevStartXmit, &link.TracepointOptions{})
	if err != nil {
		return err
	}
	p.links = append(p.links, progStart)

	progXmit, err := link.Tracepoint("net", "net_dev_xmit", p.objs.NetDevXmit, &link.TracepointOptions{})
	if err != nil {
		return err
	}
	p.links = append(p.links, progXmit)

	return nil
}
