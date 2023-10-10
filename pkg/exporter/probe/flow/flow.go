package flow

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS bpf ../../../../bpf/flow.c -- -I../../../../bpf/headers -D__TARGET_ARCH_x86

type direction int

const (
	ingress direction = 0
	egress  direction = 1

	metricsBytes   = "bytes"
	metricsPackets = "packets"
)

var (
	probeName = "flow"
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
}

type flowArgs struct {
	Dev string
}

func metricsProbeCreator(args flowArgs) (probe.MetricsProbe, error) {
	if args.Dev == "" {
		args.Dev = "eth0"
	}

	p := &metricsProbe{
		args: args,
	}
	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: []string{"protocol", "src", "dst", "sport", "dport"},
		SingleMetricsOpts: []probe.SingleMetricsOpts{
			{Name: metricsBytes, ValueType: prometheus.CounterValue},
			{Name: metricsPackets, ValueType: prometheus.CounterValue},
		},
	}
	batchMetrics := probe.NewBatchMetrics(opts, p.collectOnce)
	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type metricsProbe struct {
	bpfObjs bpfObjects
	args    flowArgs
}

func (p *metricsProbe) Start(_ context.Context) error {
	//TODO watch every netns create/destroy
	if err := p.loadAndAttachBPF(); err != nil {
		var verifierError *ebpf.VerifierError
		log.Error("failed load ebpf program", err)
		if errors.As(err, &verifierError) {
			log.Warn("detail", strings.Join(verifierError.Log, "\n"))
		}

		return err
	}

	return nil
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return p.cleanup()
}

func (p *metricsProbe) cleanup() error {
	//TODO only clean qdisc after replace qdisc successfully
	link, err := netlink.LinkByName(p.args.Dev)
	if err == nil {
		_ = cleanQdisc(link)
	}
	return p.bpfObjs.Close()
}

func toIPString(addr uint32) string {
	var bytes [4]byte
	bytes[0] = byte(addr & 0xff)
	bytes[1] = byte(addr >> 8 & 0xff)
	bytes[2] = byte(addr >> 16 & 0xff)
	bytes[3] = byte(addr >> 24 & 0xff)
	return fmt.Sprintf("%d.%d.%d.%d", bytes[0], bytes[1], bytes[2], bytes[3])
}

func (p *metricsProbe) collectOnce(emit probe.Emit) error {
	htons := func(port uint16) uint16 {
		data := make([]byte, 2)
		binary.BigEndian.PutUint16(data, port)
		return binary.LittleEndian.Uint16(data)
	}
	var values []bpfFlowMetrics
	var key bpfFlowTuple4
	iterator := p.bpfObjs.bpfMaps.InspFlow4Metrics.Iterate()

	for iterator.Next(&key, &values) {
		if err := iterator.Err(); err != nil {
			return fmt.Errorf("failed read bpfmap, err: %w", err)
		}

		var val bpfFlowMetrics
		for i := 0; i < len(values); i++ {
			val.Bytes += values[i].Bytes
			val.Packets += values[i].Packets
		}

		var protocol string

		switch key.Proto {
		case 1:
			protocol = "icmp"
		case 6:
			protocol = "tcp"
		case 17:
			protocol = "udp"
		case 132:
			protocol = "sctp"
		default:
			log.Errorf("%s unknown ip protocol number %d", probeName, key.Proto)
		}

		labels := []string{
			protocol,
			toIPString(key.Src),
			toIPString(key.Dst),
			fmt.Sprintf("%d", htons(key.Sport)),
			fmt.Sprintf("%d", htons(key.Dport)),
		}

		emit("bytes", labels, float64(val.Bytes))
		emit("packets", labels, float64(val.Packets))
	}
	return nil
}

func (p *metricsProbe) setupTCFilter(link netlink.Link) error {
	if err := replaceQdisc(link); err != nil {
		return errors.Wrapf(err, "failed replace qdics clsact for dev %s", link.Attrs().Name)
	}

	replaceFilter := func(direction direction) error {
		directionName := ""
		var filterParent uint32
		var prog *ebpf.Program
		switch direction {
		case ingress:
			directionName = "ingress"
			filterParent = netlink.HANDLE_MIN_INGRESS
			prog = p.bpfObjs.bpfPrograms.TcIngress
		case egress:
			directionName = "egress"
			filterParent = netlink.HANDLE_MIN_EGRESS
			prog = p.bpfObjs.bpfPrograms.TcEgress
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

	if err := replaceFilter(ingress); err != nil {
		return errors.Wrapf(err, "cannot set ingress filter for dev %s", link.Attrs().Name)
	}
	if err := replaceFilter(egress); err != nil {
		return errors.Wrapf(err, "cannot set egress filter for dev %s", link.Attrs().Name)
	}
	return nil
}

func (p *metricsProbe) loadBPF() error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove limit failed: %s", err.Error())
	}

	opts := ebpf.CollectionOptions{}

	opts.Programs = ebpf.ProgramOptions{
		LogLevel:    ebpf.LogLevelInstruction | ebpf.LogLevelBranch | ebpf.LogLevelStats,
		KernelTypes: bpfutil.LoadBTFSpecOrNil(),
	}

	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&p.bpfObjs, &opts); err != nil {
		return fmt.Errorf("failed loading objects: %w", err)
	}
	return nil
}

func (p *metricsProbe) loadAndAttachBPF() error {
	eth0, err := netlink.LinkByName(p.args.Dev)
	if err != nil {
		return fmt.Errorf("fail get link %s, err: %w", p.args.Dev, err)
	}

	if err := p.loadBPF(); err != nil {
		return err
	}

	if err := p.setupTCFilter(eth0); err != nil {
		return fmt.Errorf("failed replace %s qdisc with clsact, err: %v", p.args.Dev, err)
	}
	return nil
}

func cleanQdisc(link netlink.Link) error {
	return netlink.QdiscDel(clsact(link))
}

func clsact(link netlink.Link) netlink.Qdisc {
	attrs := netlink.QdiscAttrs{
		LinkIndex: link.Attrs().Index,
		Handle:    netlink.MakeHandle(0xffff, 0),
		Parent:    netlink.HANDLE_CLSACT,
	}

	return &netlink.GenericQdisc{
		QdiscAttrs: attrs,
		QdiscType:  "clsact",
	}
}

func replaceQdisc(link netlink.Link) error {
	return netlink.QdiscReplace(clsact(link))
}
