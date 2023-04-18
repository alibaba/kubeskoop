package tracenetif

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/bits"
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

// nolint
//
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_nftxlat_event_t -type insp_nftxlat_metric_t bpf ../../../../bpf/netiftxlatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86
const (
	TXLAT_QDISC_SLOW  = "netiftxlat_qdiscslow100ms"
	TXLAT_NETDEV_SLOW = "netiftxlat_netdevslow100ms"
)

var (
	ModuleName = "insp_netiftxlat" // nolint

	probe      = &NetifTxlatencyProbe{once: sync.Once{}}
	links      = []link.Link{}
	events     = []string{"TXLAT_QDISC_100MS", "TXLAT_NETDEV_100MS"}
	metrics    = []string{TXLAT_QDISC_SLOW, TXLAT_NETDEV_SLOW}
	metricsMap = map[string]map[uint32]uint64{}

	perfReader *perf.Reader
)

func GetProbe() *NetifTxlatencyProbe {
	return probe
}

func init() {
	for m := range metrics {
		metricsMap[metrics[m]] = map[uint32]uint64{}
	}
}

type NetifTxlatencyProbe struct {
	enable bool
	once   sync.Once
	sub    chan<- proto.RawEvent
	mtx    sync.Mutex
}

func (p *NetifTxlatencyProbe) Name() string {
	return ModuleName
}

func (p *NetifTxlatencyProbe) Start(ctx context.Context) {
	// 将eBPF程序进行link
	p.once.Do(func() {
		err := start()
		if err != nil {
			slog.Ctx(ctx).Warn("start", "module", ModuleName, "err", err)
			return
		}
		p.enable = true
	})

	if !p.enable {
		// if load failed, do nat start process
		return
	}

	slog.Debug("start probe", "module", ModuleName)
	if perfReader == nil {
		slog.Ctx(ctx).Warn("start", "module", ModuleName, "err", "perf reader not ready")
		return
	}
	// 开始针对perf事件进行读取
	for {
		record, err := perfReader.Read()
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

		// 解析perf事件信息，输出为proto.RawEvent
		var event bpfInspNftxlatEventT
		// Parse the ringbuf event entry into a bpfEvent structure.
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "err", err)
			continue
		}

		rawevt := proto.RawEvent{
			Netns: event.SkbMeta.Netns,
		}
		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		rawevt.EventBody = fmt.Sprintf("%s latency:%s", tuple, bpfutil.GetHumanTimes(event.Latency))
		/*#define THRESH
		#define ACTION_QDISC	    1
		#define ACTION_XMIT	        2
		*/
		if event.Type == 1 {
			rawevt.EventType = "NETIFTXLAT_QDISC"
			p.updateMetrics(rawevt.Netns, TXLAT_QDISC_SLOW)
		} else if event.Type == 2 {
			rawevt.EventType = "NETIFTXLAT_XMIT"
			p.updateMetrics(rawevt.Netns, TXLAT_NETDEV_SLOW)
		}

		// 分发给注册的dispatcher，其余逻辑由框架完成
		if p.sub != nil {
			slog.Ctx(ctx).Debug("broadcast event", "module", ModuleName)
			p.sub <- rawevt
		}
	}
}

// Register register sub chan to get perf events
func (p *NetifTxlatencyProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *NetifTxlatencyProbe) Ready() bool {
	return p.enable
}

func (p *NetifTxlatencyProbe) Close() error {
	if p.enable {
		for _, link := range links {
			link.Close()
		}
		links = []link.Link{}
	}

	if perfReader != nil {
		perfReader.Close()
		perfReader = nil
	}

	return nil
}

func (p *NetifTxlatencyProbe) updateMetrics(netns uint32, metric string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if _, ok := metricsMap[metric]; ok {
		metricsMap[metric][netns]++
	}
}

func (p *NetifTxlatencyProbe) GetEventNames() []string {
	return events
}

func (p *NetifTxlatencyProbe) GetMetricNames() []string {
	return metrics
}

func (p *NetifTxlatencyProbe) Collect(_ context.Context) (map[string]map[uint32]uint64, error) {
	ets := nettop.GetAllEntity()
	resMap := map[string]map[uint32]uint64{}

	for metric, v := range metricsMap {
		resMap[metric] = make(map[uint32]uint64)
		for _, et := range ets {
			if et != nil {
				nsinum := et.GetNetns()
				if v, ok := v[uint32(nsinum)]; ok {
					resMap[metric][uint32(nsinum)] = v
				} else {
					// if no kernel latency event recorded, set value to 0
					resMap[metric][uint32(nsinum)] = 0
				}
			}
		}
	}
	return resMap, nil
}

func start() error {
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
	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %v", err)
	}

	// 执行link操作，保存rawfd
	progQueue, err := link.Tracepoint("net", "net_dev_queue", objs.NetDevQueue, &link.TracepointOptions{})
	if err != nil {
		return err
	}
	links = append(links, progQueue)

	progStart, err := link.Tracepoint("net", "net_dev_start_xmit", objs.NetDevStartXmit, &link.TracepointOptions{})
	if err != nil {
		return err
	}
	links = append(links, progStart)

	progXmit, err := link.Tracepoint("net", "net_dev_xmit", objs.NetDevXmit, &link.TracepointOptions{})
	if err != nil {
		return err
	}
	links = append(links, progXmit)

	// 初始化map的读接口
	reader, err := perf.NewReader(objs.bpfMaps.InspSklatEvent, int(unsafe.Sizeof(bpfInspNftxlatEventT{})))
	if err != nil {
		return err
	}
	perfReader = reader

	return nil
}
