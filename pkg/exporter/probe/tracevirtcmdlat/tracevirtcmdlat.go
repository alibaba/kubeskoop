package tracevirtcmdlat

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_virtcmdlat_event_t  bpf ../../../../bpf/virtcmdlatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

const (
	ModuleName = "insp_virtcmdlatency" // nolint

	VIRTCMD100MS  = "virtcmdlatency100ms"
	VIRTCMD       = "virtcmdlatency"
	VIRTCMDEXCUTE = "VIRTCMDEXCUTE"

	fn = "virtnet_send_command"
)

var (
	probe   = &VirtcmdLatencyProbe{once: sync.Once{}, mtx: sync.Mutex{}}
	objs    = bpfObjects{}
	links   = []link.Link{}
	events  = []string{VIRTCMDEXCUTE}
	metrics = []string{VIRTCMD100MS, VIRTCMD}

	metricsMap = map[string]map[uint32]uint64{}
)

func GetProbe() *VirtcmdLatencyProbe {
	return probe
}

func init() {
	for m := range metrics {
		metricsMap[metrics[m]] = map[uint32]uint64{
			0: 0,
		}
	}
}

type VirtcmdLatencyProbe struct {
	enable bool
	sub    chan<- proto.RawEvent
	once   sync.Once
	mtx    sync.Mutex
}

func (p *VirtcmdLatencyProbe) Name() string {
	return ModuleName
}

func (p *VirtcmdLatencyProbe) Ready() bool {
	return p.enable
}

func (p *VirtcmdLatencyProbe) GetMetricNames() []string {
	return metrics
}

func (p *VirtcmdLatencyProbe) GetEventNames() []string {
	return events
}

func (p *VirtcmdLatencyProbe) Close() error {
	if p.enable {
		for _, link := range links {
			link.Close()
		}
		links = []link.Link{}
	}

	return nil
}

func (p *VirtcmdLatencyProbe) Collect(_ context.Context) (map[string]map[uint32]uint64, error) {
	return metricsMap, nil
}

func (p *VirtcmdLatencyProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *VirtcmdLatencyProbe) Start(ctx context.Context) {
	// metric and events both start probe
	if p.enable {
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
		// if load failed, do nat start process
		return
	}

	reader, err := perf.NewReader(objs.bpfMaps.InspVirtcmdlatEvents, int(unsafe.Sizeof(bpfInspVirtcmdlatEventT{})))
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

		var event bpfInspVirtcmdlatEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "err", err)
			continue
		}

		rawevt := proto.RawEvent{
			Netns:     0,
			EventType: VIRTCMDEXCUTE,
		}

		if event.Latency > 100000000 {
			p.updateMetrics(VIRTCMD100MS)
		} else {
			p.updateMetrics(VIRTCMD)
		}

		rawevt.EventBody = fmt.Sprintf("cpu=%d  pid=%d  latency=%s", event.Cpu, event.Pid, bpfutil.GetHumanTimes(event.Latency))
		if p.sub != nil {
			slog.Ctx(ctx).Debug("broadcast event", "module", ModuleName)
			p.sub <- rawevt
		}
	}
}

func (p *VirtcmdLatencyProbe) updateMetrics(k string) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if _, ok := metricsMap[k]; ok {
		metricsMap[k][0]++
	}
}

func loadSync() error {
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

	linkentry, err := link.Kprobe(fn, objs.TraceVirtcmd, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link %s: %s", fn, err.Error())
	}
	links = append(links, linkentry)

	linkexit, err := link.Kretprobe(fn, objs.TraceVirtcmdret, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link ret %s: %s", fn, err.Error())
	}

	links = append(links, linkexit)
	return nil
}
