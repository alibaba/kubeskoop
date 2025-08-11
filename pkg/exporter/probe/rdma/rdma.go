package rdma

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
)

const (
	probeName       = "rdma"
	linkTypeUnknown = "unknown"

	ibDefaultBasePath = "/sys/class/infiniband"
)

var (
	resourceSummaryEntries = []string{"cm_id", "cq", "ctx", "mr", "pd", "qp", "srq"}
	rdmaDevLabels          = []string{"device", "type"}
	rdmaDevPortLabels      = append(rdmaDevLabels, "port")
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
}

func metricsProbeCreator() (probe.MetricsProbe, error) {
	nlHandle, err := netlink.NewHandle()
	if err != nil {
		return nil, err
	}
	p := &metricsProbe{
		netlink: nlHandle,
	}

	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: probe.StandardMetricsLabels,
		SingleMetricsOpts: lo.Map(resourceSummaryEntries, func(entry string, _ int) probe.SingleMetricsOpts {
			return probe.SingleMetricsOpts{Name: entry, VariableLabels: rdmaDevLabels, Help: fmt.Sprintf("rdma resource summary %s", entry), ValueType: prometheus.GaugeValue}
		}),
	}
	opts.SingleMetricsOpts = append(opts.SingleMetricsOpts, mlx5Metrics...)
	opts.SingleMetricsOpts = append(opts.SingleMetricsOpts, erdmaMetrics...)
	batchMetrics := probe.NewBatchMetrics(opts, p.collectOnce)
	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

type netlinkInterface interface {
	RdmaResourceList() ([]*netlink.RdmaResource, error)
	RdmaLinkByName(name string) (*netlink.RdmaLink, error)
	RdmaStatistic(link *netlink.RdmaLink) (*netlink.RdmaDeviceStatistic, error)
}

type metricsProbe struct {
	netlink  netlinkInterface
	basePath string
}

func (p *metricsProbe) Start(_ context.Context) error {
	return nil
}

func (p *metricsProbe) Stop(_ context.Context) error {
	return nil
}

func (p *metricsProbe) collectOnce(emit probe.Emit) error {
	// rdma only collect host network
	entity, err := nettop.GetHostNetworkEntity()
	if err != nil {
		return err
	}
	rdmaRes, err := p.netlink.RdmaResourceList()
	if err != nil {
		return err
	}
	if len(rdmaRes) == 0 {
		return nil
	}
	standardLabelValues := probe.BuildStandardMetricsLabelValues(entity)
	for _, res := range rdmaRes {
		link, err := p.netlink.RdmaLinkByName(res.Name)
		if err != nil {
			log.Errorf("failed get rdma link %v, error: %v", res.Name, err)
			continue
		}
		linkType := rdmaLinkType(link)
		deviceLabelValues := append(standardLabelValues, res.Name, linkType)
		for resKey, resVal := range res.RdmaResourceSummaryEntries {
			emit(resKey, deviceLabelValues, float64(resVal))
		}
		if linkType == "unknown" {
			continue
		}

		linkStatistics, err := p.netlink.RdmaStatistic(link)
		if err != nil {
			log.Errorf("failed get rdma statistics %v, error: %v", res.Name, err)
			continue
		}

		for _, port := range linkStatistics.RdmaPortStatistics {
			devicePortLabelValues := append(deviceLabelValues, strconv.FormatUint(uint64(port.PortIndex), 10))
			for statKey, statVal := range port.Statistics {
				emit(strings.Join([]string{linkType, statKey}, "_"), devicePortLabelValues, float64(statVal))
			}
		}

		linkCounterStatistics, err := metricsFromSysFS(link, p.basePath)
		if err != nil {
			log.Warnf("failed get rdma counter statistics %v, error: %v", res.Name, err)
			continue
		}
		for _, port := range linkCounterStatistics {
			devicePortLabelValues := append(deviceLabelValues, strconv.FormatUint(uint64(port.PortIndex), 10))
			for statKey, statVal := range port.Statistics {
				emit(strings.Join([]string{linkType, statKey}, "_"), devicePortLabelValues, float64(statVal))
			}
		}
	}
	return nil
}

func metricsFromSysFS(link *netlink.RdmaLink, basePath string) ([]netlink.RdmaPortStatistic, error) {
	if basePath == "" {
		basePath = ibDefaultBasePath
	}
	if link == nil {
		return nil, fmt.Errorf("rdma link is nil")
	}
	devPathPorts := filepath.Join(basePath, link.Attrs.Name, "ports")
	ports, err := os.ReadDir(devPathPorts)
	if err != nil {
		return nil, fmt.Errorf("failed read rdma ports %s: %w", devPathPorts, err)
	}
	var ret []netlink.RdmaPortStatistic
	for _, port := range ports {
		portCountersPath := filepath.Join(devPathPorts, port.Name(), "counters")
		portIndex, err := strconv.ParseUint(port.Name(), 10, 32)
		if err != nil {
			log.Errorf("failed parse rdma port index %s: %v", port.Name(), err)
			continue
		}
		portCounters, err := os.ReadDir(portCountersPath)
		if err != nil {
			log.Errorf("failed read rdma port counters %s: %v", portCountersPath, err)
			continue
		}
		portStatistics := netlink.RdmaPortStatistic{
			PortIndex:  uint32(portIndex),
			Statistics: make(map[string]uint64),
		}
		for _, counter := range portCounters {
			counterPath := filepath.Join(portCountersPath, counter.Name())
			counterVal, err := os.ReadFile(counterPath)
			if err != nil {
				log.Errorf("failed read rdma port counter %s: %v", counterPath, err)
				continue
			}
			counterValUint, err := strconv.ParseUint(strings.TrimSpace(string(counterVal)), 10, 64)
			if err != nil {
				log.Errorf("failed parse rdma port counter %s: %v", counterPath, err)
				continue
			}
			portStatistics.Statistics[counter.Name()] = counterValUint
		}
		ret = append(ret, portStatistics)
	}

	return ret, nil
}

func rdmaLinkType(link *netlink.RdmaLink) string {
	if link == nil {
		return linkTypeUnknown
	}
	switch strings.Split(link.Attrs.Name, "_")[0] {
	case linkTypeMellanox:
		return linkTypeMellanox
	case linkTypeERdma:
		return linkTypeERdma
	default:
		return linkTypeUnknown
	}
}
