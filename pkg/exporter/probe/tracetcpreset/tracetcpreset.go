package tracetcpreset

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"strings"
	"sync"
	"syscall"
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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_tcpreset_event_t bpf ../../../../bpf/tcpreset.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	TCPRESET_NOSOCK  = "TCPRESET_NOSOCK"
	TCPRESET_ACTIVE  = "TCPRESET_ACTIVE"
	TCPRESET_PROCESS = "TCPRESET_PROCESS"
	TCPRESET_RECEIVE = "TCPRESET_RECEIVE"
)

var (
	ModuleName = "insp_tcpreset" // nolint
	objs       = bpfObjects{}
	probe      = &TCPResetProbe{once: sync.Once{}, mtx: sync.Mutex{}}
	links      = []link.Link{}

	events = []string{TCPRESET_NOSOCK, TCPRESET_ACTIVE, TCPRESET_PROCESS, TCPRESET_RECEIVE}
)

func GetProbe() *TCPResetProbe {
	return probe
}

type TCPResetProbe struct {
	enable bool
	sub    chan<- proto.RawEvent
	once   sync.Once
	mtx    sync.Mutex
}

func (p *TCPResetProbe) Name() string {
	return ModuleName
}

func (p *TCPResetProbe) Ready() bool {
	return p.enable
}

func (p *TCPResetProbe) GetEventNames() []string {
	return events
}

func (p *TCPResetProbe) Close() error {
	if p.enable {
		for _, link := range links {
			link.Close()
		}
		links = []link.Link{}
	}

	return nil
}

func (p *TCPResetProbe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func (p *TCPResetProbe) Start(ctx context.Context) {
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

	reader, err := perf.NewReader(objs.bpfMaps.InspTcpresetEvents, int(unsafe.Sizeof(bpfInspTcpresetEventT{})))
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

		var event bpfInspTcpresetEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "err", err)
			continue
		}

		rawevt := proto.RawEvent{
			Netns: 0,
		}

		/*
			#define RESET_NOSOCK 1
			#define RESET_ACTIVE 2
			#define RESET_PROCESS 4
			#define RESET_RECEIVE 8
		*/
		switch event.Type {
		case 1:
			rawevt.EventType = TCPRESET_NOSOCK
		case 2:
			rawevt.EventType = TCPRESET_ACTIVE
		case 4:
			rawevt.EventType = TCPRESET_PROCESS
		case 8:
			rawevt.EventType = TCPRESET_RECEIVE
		default:
			slog.Ctx(ctx).Info("parsing event", "module", ModuleName, "ignore", event)
		}

		rawevt.Netns = event.SkbMeta.Netns
		if event.Tuple.L3Proto == syscall.ETH_P_IPV6 {
			slog.Ctx(ctx).Debug("ignore event of ipv6 proto")
			continue
		}
		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		stateStr := bpfutil.GetSkcStateStr(event.State)
		rawevt.EventBody = fmt.Sprintf("%s state:%s ", tuple, stateStr)
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
		if strings.Contains(err.Error(), "no BTF found for kernel") {
			_BpfBytes, err = bpfutil.CompileBPF("tcpreset")
			if err != nil {
				return err
			}
			opts.Programs.KernelTypes = nil
			err = loadBpfObjects(&objs, &opts)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("loading objects: %v", err)
		}
	}

	progsend, err := link.Kprobe("tcp_v4_send_reset", objs.TraceSendreset, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link tcp_v4_send_reset: %s", err.Error())
	}
	links = append(links, progsend)

	progactive, err := link.Kprobe("tcp_send_active_reset", objs.TraceSendactive, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link tcp_send_active_reset: %s", err.Error())
	}
	links = append(links, progactive)

	kprecv, err := link.Tracepoint("tcp", "tcp_receive_reset", objs.InspRstrx, nil)
	if err != nil {
		return err
	}
	links = append(links, kprecv)

	return nil
}
