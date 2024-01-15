package info

import (
	"context"

	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	probeName = "info"

	metricsPod  = "pod"
	metricsNode = "node"
)

type metricsProbe struct {
	nodeIPs []string
}

func metricsProbeCreator() (probe.MetricsProbe, error) {
	nodeIPs, err := nettop.GetNodeIPs()
	if err != nil {
		return nil, err
	}
	p := &metricsProbe{
		nodeIPs: nodeIPs,
	}

	opts := probe.BatchMetricsOpts{
		Namespace:      probe.MetricsNamespace,
		Subsystem:      probeName,
		VariableLabels: []string{"node_name", "ip"},
		SingleMetricsOpts: []probe.SingleMetricsOpts{
			{Name: metricsPod, ValueType: prometheus.GaugeValue, VariableLabels: []string{"pod_name", "pod_namespace"}},
			{Name: metricsNode, ValueType: prometheus.GaugeValue},
		},
	}

	batchMetrics := probe.NewBatchMetrics(opts, p.collectOnce)
	return probe.NewMetricsProbe(probeName, p, batchMetrics), nil
}

func (m *metricsProbe) collectOnce(emit probe.Emit) error {
	nodeName := nettop.GetNodeName()
	ets := nettop.GetAllEntity()
	for _, ip := range m.nodeIPs {
		emit(metricsNode, []string{nodeName, ip}, 1)
	}

	podsMap := make(map[int]bool)
	for _, et := range ets {
		if et.IsHostNetwork() || et.GetIP() == "" {
			continue
		}
		if podsMap[et.GetNetns()] {
			continue
		}
		podsMap[et.GetNetns()] = true
		labels := []string{nodeName, et.GetIP(), et.GetPodName(), et.GetPodNamespace()}
		emit(metricsPod, labels, 1)
	}
	return nil
}

func (m *metricsProbe) Start(_ context.Context) error {
	return nil
}

func (m *metricsProbe) Stop(_ context.Context) error {
	return nil
}

func init() {
	probe.MustRegisterMetricsProbe("info", metricsProbeCreator)
}
