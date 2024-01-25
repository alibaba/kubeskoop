package tracesocketlatency

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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_sklat_metric_t -type insp_sklat_event_t bpf ../../../../bpf/socketlatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	ModuleName = "socketlatency" // nolint

	SOCKETLAT_READSLOW = "SOCKETLAT_READSLOW"
	SOCKETLAT_SENDSLOW = "SOCKETLAT_SENDSLOW"

	READ100MS  = "socketlatencyread100ms"
	READ300MS  = "socketlatencyread300ms"
	READ1MS    = "socketlatencyread1ms"
	WRITE100MS = "socketlatencywrite100ms"
	WRITE1MS   = "socketlatencywrite1ms"

	/*
		#define ACTION_READ	    1
		#define ACTION_WRITE	2
		#define ACTION_HANDLE	4

		#define BUCKET100MS 1
		#define BUCKET10MS  2
		#define BUCKET1MS   4
	*/
	ACTION_READ   = 1
	ACTION_WRITE  = 2
	ACTION_HANDLE = 4

	BUCKET100MS = 1
	BUCKET1MS   = 4
)

var (
	probeName            = "socketlatency"
	_socketLatency       = &socketLatencyProbe{}
	socketlatencyMetrics = []string{READ100MS, READ1MS, WRITE100MS, WRITE1MS}
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}

func metricsProbeCreator() (probe.MetricsProbe, error) {
	p := &metricsProbe{}
	batchMetrics := probe.NewLegacyBatchMetrics(probeName, socketlatencyMetrics, p.CollectOnce)

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
	return _socketLatency.start(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return _socketLatency.stop(probe.ProbeTypeMetrics)
}

func (p *metricsProbe) CollectOnce() (map[string]map[uint32]uint64, error) {
	return _socketLatency.collect()
}

type eventProbe struct {
	sink chan<- *probe.Event
}

func (e *eventProbe) Start(_ context.Context) error {
	err := _socketLatency.start(probe.ProbeTypeEvent)
	if err != nil {
		return err
	}

	_socketLatency.sink = e.sink
	return nil
}

func (e *eventProbe) Stop(_ context.Context) error {
	return _socketLatency.stop(probe.ProbeTypeEvent)
}

type socketLatencyProbe struct {
	objs       bpfObjects
	links      []link.Link
	sink       chan<- *probe.Event
	refcnt     [probe.ProbeTypeCount]int
	lock       sync.Mutex
	perfReader *perf.Reader
}

func (p *socketLatencyProbe) stop(probeType probe.Type) error {
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

func (p *socketLatencyProbe) cleanup() error {
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

func (p *socketLatencyProbe) totalReferenceCountLocked() int {
	var c int
	for _, n := range p.refcnt {
		c += n
	}
	return c
}

func (p *socketLatencyProbe) start(probeType probe.Type) (err error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.refcnt[probeType]++
	if p.totalReferenceCountLocked() == 1 {
		if err = p.loadAndAttachBPF(); err != nil {
			log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
			_ = p.cleanup()
			return
		}
		p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspSklatEvents, int(unsafe.Sizeof(bpfInspSklatEventT{})))
		if err != nil {
			log.Warnf("%s failed create new perf reader, err: %v", probeName, err)
			return
		}

		go p.perfLoop()
	}

	return nil
}

func (p *socketLatencyProbe) perfLoop() {
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

		var event bpfInspSklatEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Infof("%s failed parsing event, err: %v", probeName, err)
			continue
		}
		// filter netlink/unixsock/tproxy packet
		if event.Tuple.Dport == 0 && event.Tuple.Sport == 0 {
			continue
		}
		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Labels:    probe.LagacyEventLabels(event.SkbMeta.Netns),
		}
		/*
			#define ACTION_READ	    1
			#define ACTION_WRITE	2
		*/
		if event.Direction == ACTION_READ {
			evt.Type = SOCKETLAT_READSLOW
		} else if event.Direction == ACTION_WRITE {
			evt.Type = SOCKETLAT_SENDSLOW
		}

		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		evt.Message = fmt.Sprintf("%s latency=%s", tuple, bpfutil.GetHumanTimes(event.Latency))
		if p.sink != nil {
			log.Debugf("%s sink event %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}
}

func (p *socketLatencyProbe) collect() (map[string]map[uint32]uint64, error) {
	res := map[string]map[uint32]uint64{}
	for _, mtr := range socketlatencyMetrics {
		res[mtr] = map[uint32]uint64{}
	}
	// 从map中获取数据
	m, err := bpfutil.MustLoadPin(ModuleName)
	if err != nil {
		return nil, err
	}

	var (
		value   uint64
		entries = m.Iterate()
		key     = bpfInspSklatMetricT{}
	)
	// 解析数据后更新指标，按照指标/netns/数据的格式存放map[string]map[uint32]uint64
	for entries.Next(&key, &value) {
		if key.Netns == 0 {
			continue
		}

		if key.Action == ACTION_READ {
			if key.Bucket == BUCKET100MS {
				res[READ100MS][key.Netns] += value
			} else if key.Bucket == BUCKET1MS {
				res[READ1MS][key.Netns] += value
			}
		}

		if key.Action == ACTION_WRITE {
			if key.Bucket == BUCKET100MS {
				res[WRITE100MS][key.Netns] += value
			} else if key.Bucket == BUCKET1MS {
				res[WRITE1MS][key.Netns] += value
			}
		}
	}

	return res, nil
}

func (p *socketLatencyProbe) loadAndAttachBPF() error {
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

	linkcreate, err := link.Kprobe("inet_ehash_nolisten", p.objs.SockCreate, nil)
	if err != nil {
		return fmt.Errorf("link inet_ehash_nolisten: %s", err.Error())
	}
	p.links = append(p.links, linkcreate)

	linkreceive, err := link.Kprobe("sock_def_readable", p.objs.SockReceive, nil)
	if err != nil {
		return fmt.Errorf("link sock_def_readable: %s", err.Error())
	}
	p.links = append(p.links, linkreceive)

	linkread, err := link.Kprobe("tcp_cleanup_rbuf", p.objs.SockRead, nil)
	if err != nil {
		return fmt.Errorf("link tcp_cleanup_rbuf: %s", err.Error())
	}
	p.links = append(p.links, linkread)

	linkwrite, err := link.Kprobe("tcp_sendmsg_locked", p.objs.SockWrite, nil)
	if err != nil {
		return fmt.Errorf("link tcp_sendmsg_locked: %s", err.Error())
	}
	p.links = append(p.links, linkwrite)

	linksend, err := link.Kprobe("tcp_write_xmit", p.objs.SockSend, nil)
	if err != nil {
		return fmt.Errorf("link tcp_write_xmit: %s", err.Error())
	}
	p.links = append(p.links, linksend)

	linkdestroy, err := link.Kprobe("tcp_done", p.objs.SockDestroy, nil)
	if err != nil {
		return fmt.Errorf("link tcp_done: %s", err.Error())
	}
	p.links = append(p.links, linkdestroy)

	err = bpfutil.MustPin(p.objs.InspSklatMetric, probeName)
	if err != nil {
		return fmt.Errorf("pin map %s failed: %s", ModuleName, err.Error())
	}

	return nil
}
