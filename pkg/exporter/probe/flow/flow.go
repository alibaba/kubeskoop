package flow

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	log "golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS bpf ../../../../bpf/flow.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

type Probe struct {
	enable bool
}

type Direction int

const (
	ModuleName           = "flow"
	Ingress    Direction = 0
	Egress     Direction = 1
)

var (
	probe   proto.MetricProbe = &Probe{}
	dev                       = "eth0" //TODO 通过参数指定设备名称
	bpfObjs                   = bpfObjects{}
)

func GetProbe() proto.MetricProbe {
	return probe

}
func (f *Probe) Start(_ context.Context, _ proto.ProbeType) {
	log.Info("flow probe starting...")

	eth0, err := netlink.LinkByName(dev)
	if err != nil {
		log.Error("fail get link eth0", err)
		return
	}

	if err := load(); err != nil {
		var verifierError *ebpf.VerifierError
		log.Error("failed load ebpf program", err)
		if errors.As(err, &verifierError) {
			log.Warn("detail", strings.Join(verifierError.Log, "\n"))
		}
		return
	}

	if err := setupTCFilter(eth0); err != nil {
		log.Error("failed replace eth0 qdisc with clsact", err)
		return
	}

	log.Info("finish setup flow ebpf")

	//below is just for testing
	//toip := func(addr uint32) string {
	//	var bytes [4]byte
	//	bytes[0] = byte(addr & 0xff)
	//	bytes[1] = byte(addr >> 8 & 0xff)
	//	bytes[2] = byte(addr >> 16 & 0xff)
	//	bytes[3] = byte(addr >> 24 & 0xff)
	//	return fmt.Sprintf("%d.%d.%d.%d", bytes[0], bytes[1], bytes[2], bytes[3])
	//}
	//htons := func(port uint16) uint16 {
	//	data := make([]byte, 2)
	//	binary.BigEndian.PutUint16(data, port)
	//	return binary.LittleEndian.Uint16(data)
	//}
	//go func() {
	//	for {
	//		var values []bpfFlowMetrics
	//		var key bpfFlowTuple4
	//		iterator := bpfObjs.bpfMaps.InspFlow4Metrics.Iterate()
	//		for {
	//			if !iterator.Next(&key, &values) {
	//				break
	//			}
	//
	//			if err := iterator.Err(); err != nil {
	//				log.Error("failed read map", err)
	//				break
	//			}
	//
	//			var val bpfFlowMetrics
	//			for i := 0; i < len(values); i++ {
	//				val.Bytes += values[i].Bytes
	//				val.Packets += values[i].Packets
	//			}
	//
	//			fmt.Printf("proto: %d %s:%d->%s:%d pkts: %d, bytes: %d\n", key.Proto, toip(key.Src), htons(key.Sport), toip(key.Dst), htons(key.Dport), val.Packets, val.Bytes)
	//		}
	//	}
	//}()

	f.enable = true
}

func setupTCFilter(link netlink.Link) error {
	if err := replaceQdisc(link); err != nil {
		return errors.Wrapf(err, "failed replace qdics clsact for dev %s", link.Attrs().Name)
	}

	replaceFilter := func(direction Direction) error {
		directionName := ""
		var filterParent uint32
		var prog *ebpf.Program
		switch direction {
		case Ingress:
			directionName = "ingress"
			filterParent = netlink.HANDLE_MIN_INGRESS
			prog = bpfObjs.bpfPrograms.TcIngress
		case Egress:
			directionName = "egress"
			filterParent = netlink.HANDLE_MIN_EGRESS
			prog = bpfObjs.bpfPrograms.TcEgress
		default:
			return fmt.Errorf("invalid direction value: %d", direction)
		}

		filter := &netlink.BpfFilter{
			FilterAttrs: netlink.FilterAttrs{
				LinkIndex: link.Attrs().Index,
				Parent:    filterParent,
				Handle:    1,
				Protocol:  unix.ETH_P_IP,
				Priority:  1,
			},
			Fd:           prog.FD(),
			Name:         fmt.Sprintf("%s-%s", link.Attrs().Name, directionName),
			DirectAction: true,
		}

		if err := netlink.FilterReplace(filter); err != nil {
			return fmt.Errorf("replace tc filter: %w", err)
		}
		return nil
	}

	if err := replaceFilter(Ingress); err != nil {
		return errors.Wrapf(err, "cannot set ingress filter for dev %s", link.Attrs().Name)
	}
	if err := replaceFilter(Egress); err != nil {
		return errors.Wrapf(err, "cannot set egress filter for dev %s", link.Attrs().Name)
	}
	return nil
}

func load() error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove limit failed: %s", err.Error())
	}

	opts := ebpf.CollectionOptions{}

	//TODO 优化btf文件的查找方式
	btf, err := bpfutil.LoadBTFFromFile("/sys/kernel/btf/vmlinux")
	if err != nil {
		panic(err)
	}

	opts.Programs = ebpf.ProgramOptions{
		LogLevel:    ebpf.LogLevelInstruction | ebpf.LogLevelBranch | ebpf.LogLevelStats,
		KernelTypes: btf,
	}

	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&bpfObjs, &opts); err != nil {
		return fmt.Errorf("failed loading objects: %w", err)
	}
	return nil
}

func replaceQdisc(link netlink.Link) error {
	attrs := netlink.QdiscAttrs{
		LinkIndex: link.Attrs().Index,
		Handle:    netlink.MakeHandle(0xffff, 0),
		Parent:    netlink.HANDLE_CLSACT,
	}

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: attrs,
		QdiscType:  "clsact",
	}

	return netlink.QdiscReplace(qdisc)
}

func (f *Probe) Close(_ proto.ProbeType) error {
	if f.enable {
		return bpfObjs.Close()
	}
	return nil
}

func (f *Probe) Ready() bool {
	return f.enable
}

func (f *Probe) Name() string {
	return ModuleName
}

func (f *Probe) GetMetricNames() []string {
	return []string{"net_flow"}
}

func (f *Probe) Collect(_ context.Context) (map[string]map[uint32]uint64, error) {
	return map[string]map[uint32]uint64{}, nil
}
