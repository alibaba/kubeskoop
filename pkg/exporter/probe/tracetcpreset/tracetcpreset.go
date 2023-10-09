package tracetcpreset

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"syscall"
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

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS -type insp_tcpreset_event_t bpf ../../../../bpf/tcpreset.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

// nolint
const (
	TCPRESET_NOSOCK  = "TCPRESET_NOSOCK"
	TCPRESET_ACTIVE  = "TCPRESET_ACTIVE"
	TCPRESET_PROCESS = "TCPRESET_PROCESS"
	TCPRESET_RECEIVE = "TCPRESET_RECEIVE"
)

var (
	probeName = "tcpreset"
)

func init() {
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}

func eventProbeCreator(sink chan<- *probe.Event) (probe.EventProbe, error) {
	p := &tcpResetProbe{
		sink: sink,
	}
	return probe.NewEventProbe(probeName, p), nil
}

type tcpResetProbe struct {
	sink       chan<- *probe.Event
	objs       bpfObjects
	links      []link.Link
	perfReader *perf.Reader
}

func (p *tcpResetProbe) Start(_ context.Context) (err error) {
	err = p.loadAndAttachBPF()
	if err != nil {
		log.Errorf("%s failed load and attach bpf, err: %v", probeName, err)
		_ = p.cleanup()
		return
	}

	p.perfReader, err = perf.NewReader(p.objs.bpfMaps.InspTcpresetEvents, int(unsafe.Sizeof(bpfInspTcpresetEventT{})))
	if err != nil {
		log.Errorf("%s failed create new perf reader, err: %v", probeName, err)
		_ = p.cleanup()
		return
	}

	go p.perfLoop()
	return
}

func (p *tcpResetProbe) perfLoop() {
	for {
		record, err := p.perfReader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				log.Infof("%s received signal, exiting..", probeName)
				return
			}
			log.Errorf("%s failed reading from reader, err: %v", probeName, err)
			continue
		}

		if record.LostSamples != 0 {
			log.Infof("%s perf event ring buffer full, drop: %d", probeName, record.LostSamples)
			continue
		}

		var event bpfInspTcpresetEventT
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Infof("%s failed parsing event, err: %v", probeName, err)
			continue
		}

		/*
			#define RESET_NOSOCK 1
			#define RESET_ACTIVE 2
			#define RESET_PROCESS 4
			#define RESET_RECEIVE 8
		*/

		var eventType probe.EventType

		switch event.Type {
		case 1:
			eventType = TCPRESET_NOSOCK
		case 2:
			eventType = TCPRESET_ACTIVE
		case 4:
			eventType = TCPRESET_PROCESS
		case 8:
			eventType = TCPRESET_RECEIVE
		default:
			log.Infof("%s got invalid perf event type %d, data: %s", probeName, event.Type, util.ToJSONString(event))
			continue
		}

		if event.Tuple.L3Proto == syscall.ETH_P_IPV6 {
			log.Infof("%s ignore event of ipv6 proto", probeName)
			continue
		}

		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Type:      eventType,
			Labels:    probe.LagacyEventLabels(event.SkbMeta.Netns),
		}

		tuple := fmt.Sprintf("protocol=%s saddr=%s sport=%d daddr=%s dport=%d ", bpfutil.GetProtoStr(event.Tuple.L4Proto), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Saddr))), bits.ReverseBytes16(event.Tuple.Sport), bpfutil.GetAddrStr(event.Tuple.L3Proto, *(*[16]byte)(unsafe.Pointer(&event.Tuple.Daddr))), bits.ReverseBytes16(event.Tuple.Dport))
		stateStr := bpfutil.GetSkcStateStr(event.State)
		evt.Message = fmt.Sprintf("%s state:%s ", tuple, stateStr)
		if p.sink != nil {
			log.Debugf("%s sink event: %s", probeName, util.ToJSONString(evt))
			p.sink <- evt
		}
	}
}

func (p *tcpResetProbe) Stop(_ context.Context) error {
	return p.cleanup()
}

func (p *tcpResetProbe) cleanup() error {
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

func (p *tcpResetProbe) loadAndAttachBPF() error {
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

	progsend, err := link.Kprobe("tcp_v4_send_reset", p.objs.TraceSendreset, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link tcp_v4_send_reset: %s", err.Error())
	}
	p.links = append(p.links, progsend)

	progactive, err := link.Kprobe("tcp_send_active_reset", p.objs.TraceSendactive, &link.KprobeOptions{})
	if err != nil {
		return fmt.Errorf("link tcp_send_active_reset: %s", err.Error())
	}
	p.links = append(p.links, progactive)

	kprecv, err := link.Tracepoint("tcp", "tcp_receive_reset", p.objs.InspRstrx, nil)
	if err != nil {
		return err
	}
	p.links = append(p.links, kprecv)

	return nil
}
