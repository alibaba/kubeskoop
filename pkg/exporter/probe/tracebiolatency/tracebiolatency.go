package tracebiolatency

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS  -type insp_biolat_event_t bpf ../../../../bpf/tracebiolatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86
var (
	ModuleName = "insp_biolatency" // nolint

	probe  = &BiolatencyProbe{once: sync.Once{}}
	links  = []link.Link{}
	events = []string{"BIOLAT_10MS", "BIOLAT_100MS"}

	perfReader *perf.Reader
)

type BiolatencyProbe struct {
	enable bool
	once   sync.Once
	sub    chan<- proto.RawEvent
	mtx    sync.Mutex
}

func GetProbe() *BiolatencyProbe {
	return probe
}

func (p *BiolatencyProbe) Name() string {
	return ModuleName
}

// Register register sub chan to get perf events
func (p *BiolatencyProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *BiolatencyProbe) Ready() bool {
	return p.enable
}

func (p *BiolatencyProbe) Close(_ proto.ProbeType) error {
	if p.enable {
		for _, link := range links {
			link.Close()
		}
		links = []link.Link{}
		p.enable = false
		p.once = sync.Once{}
	}

	if perfReader != nil {
		perfReader.Close()
		perfReader = nil
	}

	return nil
}

func (p *BiolatencyProbe) GetEventNames() []string {
	return events
}

func (p *BiolatencyProbe) Start(ctx context.Context, _ proto.ProbeType) {
	p.once.Do(func() {
		err := start()
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
		var event bpfInspBiolatEventT
		// Parse the ringbuf event entry into a bpfEvent structure.
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "err", err)
			continue
		}
		pid := event.Pid
		if et, err := nettop.GetEntityByPid(int(pid)); err != nil || et == nil {
			slog.Ctx(ctx).Warn("unspecified event", "pid", pid, "task", bpfutil.GetCommString(event.Target))
			continue
		}
		rawevt := proto.RawEvent{
			EventType: "BIOLAT_10MS",
			EventBody: fmt.Sprintf("%s %d latency %s", bpfutil.GetCommString(event.Target), event.Pid, bpfutil.GetHumanTimes(event.Latency)),
		}

		// 分发给注册的dispatcher，其余逻辑由框架完成
		if p.sub != nil {
			slog.Ctx(ctx).Debug("broadcast event", "module", ModuleName)
			p.sub <- rawevt
		}
	}
}

func start() error {
	// 准备动作
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal(err)
	}

	opts := ebpf.CollectionOptions{}

	opts.Programs = ebpf.ProgramOptions{
		KernelTypes: bpfutil.LoadBTFSpecOrNil(),
	}
	objs := bpfObjects{}
	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %s", err.Error())
	}

	linkcreate, err := link.Kprobe("blk_account_io_start", objs.BiolatStart, nil)
	if err != nil {
		return fmt.Errorf("link blk_account_io_start: %s", err.Error())
	}
	links = append(links, linkcreate)

	linkdone, err := link.Kprobe("blk_account_io_done", objs.BiolatFinish, nil)
	if err != nil {
		return fmt.Errorf("link blk_account_io_done: %s", err.Error())
	}
	links = append(links, linkdone)

	reader, err := perf.NewReader(objs.InspBiolatEvts, int(unsafe.Sizeof(bpfInspBiolatEntryT{})))
	if err != nil {
		return fmt.Errorf("perf new reader failed: %s", err.Error())
	}
	perfReader = reader
	return nil

	// for {
	// 	record, err := reader.Read()
	// 	if err != nil {
	// 		if errors.Is(err, ringbuf.ErrClosed) {
	// 			log.Println("received signal, exiting..")
	// 			return err
	// 		}
	// 		log.Printf("reading from reader: %s", err)
	// 		continue
	// 	}

	// 	if record.LostSamples != 0 {
	// 		log.Printf("Perf event ring buffer full, dropped %d samples", record.LostSamples)
	// 		continue
	// 	}

	// 	var event bpfInspBiolatEventT
	// 	// Parse the ringbuf event entry into a bpfEvent structure.
	// 	if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
	// 		log.Printf("parsing event: %s", err)
	// 		continue
	// 	}

	// 	fmt.Printf("%-10s %-6d %-6s\n", bpfutil.GetCommString(event.Target), event.Pid, bpfutil.GetHumanTimes(event.Latency))
	// }
}
