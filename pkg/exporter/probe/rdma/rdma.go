package rdma

import (
	"context"
	"fmt"
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
)

var (
	resourceSummaryEntries = []string{"cm_id", "cq", "ctx", "mr", "pd", "qp"}
	rdmaDevLabels          = []string{"device", "type"}
	rdmaDevPortLabels      = append(rdmaDevLabels, "port")
)

func init() {
	probe.MustRegisterMetricsProbe(probeName, metricsProbeCreator)
}

func metricsProbeCreator() (probe.MetricsProbe, error) {
	p := &metricsProbe{}

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

type metricsProbe struct {
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
	rdmaRes, err := netlink.RdmaResourceList()
	if err != nil {
		return err
	}
	if len(rdmaRes) == 0 {
		return nil
	}
	standardLabelValues := probe.BuildStandardMetricsLabelValues(entity)
	for _, res := range rdmaRes {
		link, err := netlink.RdmaLinkByName(res.Name)
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
		linkStatistics, err := netlink.RdmaStatistic(link)
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
	}
	return nil
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
