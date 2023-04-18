package tracesocketlatency

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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_sklat_metric_t -type insp_sklat_event_t bpf ../../../../bpf/socketlatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	MODULE_NAME = "insp_socketlatency" // nolint

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
	probe  = &SocketLatencyProbe{once: sync.Once{}, mtx: sync.Mutex{}}
	objs   = bpfObjects{}
	links  = []link.Link{}
	events = []string{SOCKETLAT_READSLOW, SOCKETLAT_SENDSLOW}

	socketlatencyMetrics = []string{READ100MS, READ1MS, WRITE100MS, WRITE1MS}
)

func GetProbe() *SocketLatencyProbe {
	return probe
}

type SocketLatencyProbe struct {
	enable bool
	sub    chan<- proto.RawEvent
	once   sync.Once
	mtx    sync.Mutex
}

func (p *SocketLatencyProbe) Name() string {
	return MODULE_NAME
}

func (p *SocketLatencyProbe) Ready() bool {
	return p.enable
}

func (p *SocketLatencyProbe) GetEventNames() []string {
	return events
}

func (p *SocketLatencyProbe) Close() error {
	if p.enable {
		for _, link := range links {
			link.Close()
		}
		links = []link.Link{}
	}

	return nil
}

func (p *SocketLatencyProbe) GetMetricNames() []string {
	res := []string{}
	for _, m := range socketlatencyMetrics {
		res = append(res, strings.ToLower(m))
	}
	return res
}

func (p *SocketLatencyProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *SocketLatencyProbe) Start(ctx context.Context) {
	// metric and events both start probe
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

	if !p.enable {
		// if load failed, do nat start process
		return
	}

	p.startEventPoll(ctx)
}

func (p *SocketLatencyProbe) Collect(_ context.Context) (map[string]map[uint32]uint64, error) {
	res := map[string]map[uint32]uint64{}
	for _, mtr := range socketlatencyMetrics {
		res[mtr] = map[uint32]uint64{}
	}
	// 从map中获取数据
	m, err := bpfutil.MustLoadPin(MODULE_NAME)
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

func (p *SocketLatencyProbe) startEventPoll(ctx context.Context) {
	reader, err := perf.NewReader(objs.bpfMaps.InspSklatEvents, int(unsafe.Sizeof(bpfInspSklatEventT{})))
	if err != nil {
		slog.Ctx(ctx).Warn("start new perf reader", "module", MODULE_NAME, "err", err)
		return
	}

	for {
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

		var event bpfInspSklatEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", MODULE_NAME, "err", err)
			continue
		}
		// filter netlink/unixsock/tproxy packet
		if event.Tuple.Dport == 0 && event.Tuple.Sport == 0 {
			continue
		}
		rawevt := proto.RawEvent{
			Netns: event.SkbMeta.Netns,
		}
		/*
			#define ACTION_READ	    1
			#define ACTION_WRITE	2
		*/
		if event.Direction == ACTION_READ {
			rawevt.EventType = SOCKETLAT_READSLOW
		} else if event.Direction == ACTION_WRITE {
			rawevt.EventType = SOCKETLAT_SENDSLOW
		}

		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		rawevt.EventBody = fmt.Sprintf("%s latency=%s", tuple, bpfutil.GetHumanTimes(event.Latency))
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
		KernelTypes: bpfutil.LoadBTFSpecOrNil(),
	}

	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %s", err.Error())
	}

	linkcreate, err := link.Kprobe("inet_ehash_nolisten", objs.SockCreate, nil)
	if err != nil {
		return fmt.Errorf("link inet_ehash_nolisten: %s", err.Error())
	}
	links = append(links, linkcreate)

	linkreceive, err := link.Kprobe("sock_def_readable", objs.SockReceive, nil)
	if err != nil {
		return fmt.Errorf("link sock_def_readable: %s", err.Error())
	}
	links = append(links, linkreceive)

	linkread, err := link.Kprobe("tcp_cleanup_rbuf", objs.SockRead, nil)
	if err != nil {
		return fmt.Errorf("link tcp_cleanup_rbuf: %s", err.Error())
	}
	links = append(links, linkread)

	linkwrite, err := link.Kprobe("tcp_sendmsg_locked", objs.SockWrite, nil)
	if err != nil {
		return fmt.Errorf("link tcp_sendmsg_locked: %s", err.Error())
	}
	links = append(links, linkwrite)

	linksend, err := link.Kprobe("tcp_write_xmit", objs.SockSend, nil)
	if err != nil {
		return fmt.Errorf("link tcp_write_xmit: %s", err.Error())
	}
	links = append(links, linksend)

	linkdestroy, err := link.Kprobe("tcp_done", objs.SockDestroy, nil)
	if err != nil {
		return fmt.Errorf("link tcp_done: %s", err.Error())
	}
	links = append(links, linkdestroy)

	err = bpfutil.MustPin(objs.InspSklatMetric, MODULE_NAME)
	if err != nil {
		return fmt.Errorf("pin map %s failed: %s", MODULE_NAME, err.Error())
	}

	return nil
}
