package tracebiolatency

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	log "github.com/sirupsen/logrus"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS  -type insp_biolat_event_t bpf ../../../../bpf/tracebiolatency.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86
var (
	probeName = "biolatency"
)

func init() {
	probe.MustRegisterEventProbe(probeName, bioLatencyProbeCreator)
}

func bioLatencyProbeCreator(sink chan<- *probe.Event, _ map[string]interface{}) (probe.EventProbe, error) {
	p := &BiolatencyProbe{
		sink: sink,
	}
	return probe.NewEventProbe(probeName, p), nil
}

type BiolatencyProbe struct {
	sink   chan<- *probe.Event
	objs   bpfObjects
	links  []link.Link
	reader *perf.Reader
}

func (p *BiolatencyProbe) Start(_ context.Context) error {
	log.Debugf("start probe %s", probeName)
	if err := p.loadAndAttachBPF(); err != nil {
		_ = p.cleanup()
		return err
	}

	var err error
	p.reader, err = perf.NewReader(p.objs.InspBiolatEvts, int(unsafe.Sizeof(bpfInspBiolatEntryT{})))
	if err != nil {
		_ = p.cleanup()
		return err
	}

	go p.perf()

	// 开始针对perf事件进行读取
	return nil
}

func (p *BiolatencyProbe) perf() {
	for {
		record, err := p.reader.Read()
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

		// 解析perf事件信息，输出为proto.Event
		var event bpfInspBiolatEventT
		// Parse the ringbuf event entry into a bpfEvent structure.
		if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
			log.Infof("%s failed parsing event, err: %v", probeName, err)
			continue
		}
		pid := event.Pid
		if et, err := nettop.GetEntityByPid(int(pid)); err != nil || et == nil {
			log.Warnf("%s got unspecified event, pid: %d, task %s", probeName, pid, bpfutil.GetCommString(event.Target))
			continue
		}
		evt := &probe.Event{
			Timestamp: time.Now().UnixNano(),
			Type:      "BIOLAT_10MS",
			Message:   fmt.Sprintf("%s %d latency %s", bpfutil.GetCommString(event.Target), event.Pid, bpfutil.GetHumanTimes(event.Latency)),
		}

		p.sink <- evt
	}
}

func (p *BiolatencyProbe) Stop(_ context.Context) error {
	return p.cleanup()
}

func (p *BiolatencyProbe) cleanup() error {
	if p.reader != nil {
		p.reader.Close()
	}

	for _, link := range p.links {
		link.Close()
	}

	p.objs.Close()

	return nil
}

func (p *BiolatencyProbe) loadAndAttachBPF() error {
	// 准备动作
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal(err)
	}

	p.links = nil

	opts := ebpf.CollectionOptions{}

	opts.Programs = ebpf.ProgramOptions{
		KernelTypes: bpfutil.LoadBTFSpecOrNil(),
	}
	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&p.objs, &opts); err != nil {
		return fmt.Errorf("loading objects: %s", err.Error())
	}

	linkcreate, err := link.Kprobe("blk_account_io_start", p.objs.BiolatStart, nil)
	if err != nil {
		return fmt.Errorf("link blk_account_io_start: %s", err.Error())
	}

	p.links = append(p.links, linkcreate)

	linkdone, err := link.Kprobe("blk_account_io_done", p.objs.BiolatFinish, nil)
	if err != nil {
		return fmt.Errorf("link blk_account_io_done: %s", err.Error())
	}

	p.links = append(p.links, linkdone)
	return nil
}
