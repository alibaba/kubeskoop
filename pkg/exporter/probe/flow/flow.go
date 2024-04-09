package flow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"syscall"

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

	ClsactQdisc = "clsact"
)

var (
	probeName = "flow"
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
}

type flowArgs struct {
	Dev               string `mapstructure:"interfaceName"`
	EnablePortInLabel bool   `mapstructure:"enablePortInLabel"`
}

func getDefaultRouteDevice() (netlink.Link, error) {
	filter := &netlink.Route{
		Dst: nil,
	}
	routers, err := netlink.RouteListFiltered(syscall.AF_INET, filter, netlink.RT_FILTER_DST)
	if err != nil {
		return nil, err
	}

	if len(routers) == 0 {
		return nil, fmt.Errorf("no default route found")
	}

	if len(routers) > 1 {
		return nil, fmt.Errorf("multi default route found")
	}

	link, err := netlink.LinkByIndex(routers[0].LinkIndex)
	if err != nil {
		return nil, err
	}
	return link, nil
}

type linkFlowHelper interface {
	start() error
	stop() error
}

type dynamicLinkFlowHelper struct {
	bpfObjs *bpfObjects
	pattern string
	done    chan struct{}
	flows   map[int]*ebpfFlow
	lock    sync.Mutex
}

func (h *dynamicLinkFlowHelper) tryStartLinkFlow(link netlink.Link) {
	log.Infof("flow: try start flow on nic %s, index %d", link.Attrs().Name, link.Attrs().Index)
	if _, ok := h.flows[link.Attrs().Index]; ok {
		log.Warnf("new interface(%s) index %d already exists, skip process", link.Attrs().Name, link.Attrs().Index)
		return
	}
	flow := &ebpfFlow{
		dev:     link,
		bpfObjs: h.bpfObjs,
	}

	if err := flow.start(); err != nil {
		log.Errorf("failed start flow on dev %s", link.Attrs().Name)
		return
	}

	h.flows[link.Attrs().Index] = flow
}

func (h *dynamicLinkFlowHelper) tryStopLinkFlow(name string, index int) {
	log.Infof("flow: try stop flow on nic %s, index %d", name, index)
	flow, ok := h.flows[index]
	if !ok {
		log.Warnf("deleted interface index %d not exists, skip process", index)
		return
	}
	_ = flow.stop()
	delete(h.flows, index)
}

func (h *dynamicLinkFlowHelper) start() error {
	h.done = make(chan struct{})
	ch := make(chan netlink.LinkUpdate)
	links, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("%s error list link, err: %w", probeName, err)
	}
	for _, link := range links {
		if !strings.HasPrefix(link.Attrs().Name, h.pattern) {
			continue
		}
		h.tryStartLinkFlow(link)
	}
	go func() {
		if err := netlink.LinkSubscribe(ch, h.done); err != nil {
			log.Errorf("%s error watch link change, err: %v", probeName, err)
			close(h.done)
		}
	}()
	go func() {
		h.lock.Lock()
		defer h.lock.Unlock()
		for {
			select {
			case change := <-ch:
				if !strings.HasSuffix(change.Attrs().Name, h.pattern) {
					break
				}
				switch change.Header.Type {
				case syscall.RTM_NEWLINK:
					link, err := netlink.LinkByIndex(int(change.Index))
					if err != nil {
						log.Errorf("failed get new created link by index %d, name %s, err: %v", change.Index, change.Attrs().Name, err)
						break
					}
					h.tryStartLinkFlow(link)
				case syscall.RTM_DELLINK:
					h.tryStopLinkFlow(change.Attrs().Name, int(change.Index))
				}
			case <-h.done:
				return
			}
		}
	}()
	return nil
}

func (h *dynamicLinkFlowHelper) stop() error {
	close(h.done)
	h.lock.Lock()
	defer h.lock.Unlock()
	var first error
	for _, flow := range h.flows {
		if err := flow.stop(); err != nil {
			if first == nil {
				first = err
			}
		}
	}
	return first
}

func metricsProbeCreator(args flowArgs) (probe.MetricsProbe, error) {
	p := &metricsProbe{
		enablePort: args.EnablePortInLabel,
	}

	if args.Dev == "" {
		log.Infof("flow: auto detect network device with default route")
		dev, err := getDefaultRouteDevice()
		if err != nil {
			return nil, fmt.Errorf("fail detect default route dev, err: %w", err)
		}
		log.Infof("flow: default network device %s", dev.Attrs().Name)

		p.helper = &ebpfFlow{
			dev:     dev,
			bpfObjs: &p.bpfObjs,
		}
	} else {
		pattern := strings.TrimSuffix(args.Dev, "*")
		if pattern != args.Dev {
			log.Infof("flow: network device pattern %s", pattern)
			p.helper = &dynamicLinkFlowHelper{
				bpfObjs: &p.bpfObjs,
				pattern: pattern,
				done:    make(chan struct{}),
				flows:   make(map[int]*ebpfFlow),
			}
		} else {
			link, err := netlink.LinkByName(pattern)
			if err != nil {
				return nil, fmt.Errorf("cannot get network interface by name %s, err: %w", pattern, err)
			}

			log.Infof("flow: network device %s", pattern)
			p.helper = &ebpfFlow{
				bpfObjs: &p.bpfObjs,
				dev:     link,
			}
		}
	}

	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: probe.TupleMetricsLabels,
		SingleMetricsOpts: []probe.SingleMetricsOpts{
			{Name: metricsBytes, ValueType: prometheus.CounterValue},
			{Name: metricsPackets, ValueType: prometheus.CounterValue},
		},
	}
	batchMetrics := probe.NewBatchMetrics(opts, p.collectOnce)
	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type metricsProbe struct {
	enablePort bool
	bpfObjs    bpfObjects
	helper     linkFlowHelper
}

func (p *metricsProbe) Start(_ context.Context) error {
	if err := p.loadBPF(); err != nil {
		var verifierError *ebpf.VerifierError
		log.Error("failed load ebpf program", err)
		if errors.As(err, &verifierError) {
			log.Warn("detail", strings.Join(verifierError.Log, "\n"))
		}
		return err
	}

	return p.helper.start()
}

func (p *metricsProbe) Stop(_ context.Context) error {
	if err := p.helper.stop(); err != nil {
		return err
	}
	return p.bpfObjs.Close()
}

func toProbeTuple(t *bpfFlowTuple4) *probe.Tuple {
	return &probe.Tuple{
		Protocol: t.Proto,
		Src:      bpfutil.GetV4AddrStr(t.Src),
		Dst:      bpfutil.GetV4AddrStr(t.Dst),
		Sport:    t.Sport,
		Dport:    t.Dport,
	}
}

func (p *metricsProbe) collectOnce(emit probe.Emit) error {
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

		tuple := toProbeTuple(&key)
		if !p.enablePort {
			tuple.Dport = 0
			tuple.Sport = 0
		}

		labels := probe.BuildTupleMetricsLabels(tuple)

		emit("bytes", labels, float64(val.Bytes))
		emit("packets", labels, float64(val.Packets))
	}
	return nil
}

func (p *metricsProbe) loadBPF() error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove limit failed: %s", err.Error())
	}

	spec, err := loadBpf()
	if err != nil {
		return fmt.Errorf("failed loading bpf: %w", err)
	}

	if p.enablePort {
		m := map[string]interface{}{
			"enable_flow_port": uint8(1),
		}
		if err := spec.RewriteConstants(m); err != nil {
			return fmt.Errorf("failed rewrite constants: %w", err)
		}
	}

	opts := ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			KernelTypes: bpfutil.LoadBTFSpecOrNil(),
		},
	}

	if err := spec.LoadAndAssign(&p.bpfObjs, &opts); err != nil {
		return fmt.Errorf("loading objects: %s", err.Error())
	}

	return nil
}

type ebpfFlow struct {
	cleanQdisc bool
	dev        netlink.Link
	bpfObjs    *bpfObjects
}

func (f *ebpfFlow) start() error {
	err := f.attachBPF()
	if err != nil {
		log.Errorf("%s failed attach ebpf to dev %s, cleanup", probeName, f.dev)
		f.cleanup()
	}
	return err
}

func (f *ebpfFlow) stop() error {
	f.cleanup()
	return nil
}

func (f *ebpfFlow) cleanup() {
	clean := func(dir direction) {
		filter, err := f.getFlowFilter(dir)
		if err != nil {
			log.Errorf("%s cannot list ingress filter for dev %s: %v", probeName, f.dev.Attrs().Name, err)
			return
		}
		if err := netlink.FilterDel(filter); err != nil {
			log.Errorf("%s cannot delete ingress filter for dev %s: %v", probeName, f.dev.Attrs().Name, err)
		}

	}
	clean(ingress)
	clean(egress)

	if f.cleanQdisc {
		_ = netlink.QdiscDel(clsact(f.dev))
	}
}

func directionName(dir direction) string {
	switch dir {
	case ingress:
		return "ingress"
	case egress:
		return "egress"
	}
	return ""
}

func filterParent(dir direction) uint32 {
	switch dir {
	case ingress:
		return netlink.HANDLE_MIN_INGRESS
	case egress:
		return netlink.HANDLE_MIN_EGRESS
	}
	return 0
}

func filterName(dev string, dir direction) string {
	directionName := directionName(dir)
	return fmt.Sprintf("kubeskoop-flow-%s-%s", dev, directionName)
}

func (f *ebpfFlow) getFlowFilter(direction direction) (*netlink.BpfFilter, error) {
	filterParent := filterParent(direction)
	filterName := filterName(f.dev.Attrs().Name, direction)

	filters, err := netlink.FilterList(f.dev, filterParent)
	if err != nil {
		return nil, fmt.Errorf("failed list filters: %w", err)
	}

	for _, filter := range filters {
		if filter.Type() != "bpf" {
			continue
		}
		f, ok := filter.(*netlink.BpfFilter)
		if !ok {
			continue
		}
		if f.Name != filterName {
			continue
		}
		return f, nil
	}
	return nil, nil
}

func (f *ebpfFlow) setupTCFilter(link netlink.Link) error {
	var (
		err error
	)
	if f.cleanQdisc, err = ensureClsactQdisc(link); err != nil {
		return fmt.Errorf("failed replace qdics clsact for dev %s: %w", link.Attrs().Name, err)
	}

	setup := func(dir direction) error {
		filterParent := filterParent(dir)
		filterName := filterName(f.dev.Attrs().Name, dir)
		directionName := directionName(dir)

		var prog *ebpf.Program

		switch dir {
		case ingress:
			prog = f.bpfObjs.bpfPrograms.TcIngress
		case egress:
			prog = f.bpfObjs.bpfPrograms.TcEgress
		}

		filter := &netlink.BpfFilter{
			FilterAttrs: netlink.FilterAttrs{
				LinkIndex: link.Attrs().Index,
				Parent:    filterParent,
				Protocol:  unix.ETH_P_IP,
				Priority:  0xffff,
			},
			Fd:           prog.FD(),
			Name:         filterName,
			DirectAction: true,
		}

		oldFilter, err := f.getFlowFilter(dir)
		if err != nil {
			return fmt.Errorf("failed list %s filter for dev %s: %w", directionName, link.Attrs().Name, err)
		}
		if oldFilter != nil {
			filter.Handle = oldFilter.Handle
		}
		if err := netlink.FilterReplace(filter); err != nil {
			return fmt.Errorf("failed replace %s filter for dev %s: %w", directionName, link.Attrs().Name, err)
		}
		return nil
	}

	if err = setup(ingress); err != nil {
		return err
	}
	if err = setup(egress); err != nil {
		return err
	}

	return nil
}

func (f *ebpfFlow) attachBPF() error {
	if err := f.setupTCFilter(f.dev); err != nil {
		return fmt.Errorf("failed replace %s qdisc with clsact, err: %v", f.dev, err)
	}
	return nil
}

func clsact(link netlink.Link) netlink.Qdisc {
	attrs := netlink.QdiscAttrs{
		LinkIndex: link.Attrs().Index,
		Handle:    netlink.MakeHandle(0xffff, 0),
		Parent:    netlink.HANDLE_CLSACT,
	}

	return &netlink.GenericQdisc{
		QdiscAttrs: attrs,
		QdiscType:  ClsactQdisc,
	}
}

func ensureClsactQdisc(link netlink.Link) (bool, error) {
	qdicsList, err := netlink.QdiscList(link)
	if err != nil {
		return false, err
	}
	for _, q := range qdicsList {
		if q.Attrs().Parent == netlink.HANDLE_CLSACT && q.Type() == ClsactQdisc {
			log.Infof("got a old clsact, not create")
			return false, nil
		}
	}
	return true, netlink.QdiscAdd(clsact(link))
}
