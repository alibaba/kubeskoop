package rdma

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

func TestTrocePrometheusDescriptors(t *testing.T) {
	registry := prometheus.NewPedanticRegistry()
	collector := probe.NewBatchMetrics(probe.BatchMetricsOpts{
		Namespace: probe.MetricsNamespace, Subsystem: probeName,
		VariableLabels: probe.StandardMetricsLabels, SingleMetricsOpts: troceMetrics,
	}, func(emit probe.Emit) error {
		labels := make([]string, len(probe.StandardMetricsLabels))
		labels = append(labels, "troce_bond_0", "troce", "1")
		for name := range troceCounters {
			emit("troce_"+name, labels, 3)
		}
		return nil
	})
	require.NoError(t, registry.Register(collector))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, len(troceCounters))
}

func TestTroceUsesSharedCollection(t *testing.T) {
	root := t.TempDir()
	name := "troce_bond_0"
	path := filepath.Join(root, name, "ports", "1", "counters")
	require.NoError(t, os.MkdirAll(path, 0755))
	hardware := make(map[string]uint64)
	expected := map[string]float64{"qp": 2}
	for key := range troceCounters {
		value := uint64(123)
		if key == "port_rcv_data" {
			value = 34226688
		}
		expected["troce_"+key] = float64(value)
		switch key {
		case "packet_seq_err", "out_of_buffer", "out_of_seq", "local_ack_timeout_err",
			"rnr_nak_retry_err", "implied_nak_seq_err", "req_cqe_err", "resp_cqe_err", "duplicate_request":
			hardware[key] = value
		default:
			require.NoError(t, os.WriteFile(filepath.Join(path, key), []byte(fmt.Sprint(value)), 0644))
		}
	}
	nl := new(MockNetlink)
	nl.On("RdmaResourceList").Return([]*netlink.RdmaResource{{Name: name, RdmaResourceSummaryEntries: map[string]uint64{"qp": 2}}}, nil)
	nl.On("RdmaLinkByName", name).Return(&netlink.RdmaLink{Attrs: netlink.RdmaLinkAttrs{Name: name}}, nil)
	nl.On("RdmaStatistic", mock.Anything).Return(&netlink.RdmaDeviceStatistic{RdmaPortStatistics: []*netlink.RdmaPortStatistic{{PortIndex: 1, Statistics: hardware}}}, nil)
	p := &metricsProbe{netlink: nl, basePath: root}
	values := map[string]float64{}
	require.NoError(t, p.collectOnce(func(metric string, labels []string, value float64) {
		require.Contains(t, labels, linkTypeTroce)
		if metric != "qp" {
			require.Equal(t, "1", labels[len(labels)-1])
		}
		require.NotContains(t, values, metric, "each source must emit a unique series")
		values[metric] = value
	}))
	require.Equal(t, expected, values, "collect native hardware and port counters without unit conversion")
	nl.AssertExpectations(t)
}

func TestNetlinkFailureSkipsPortCounters(t *testing.T) {
	for _, name := range []string{"mlx5_0", "erdma_0", "troce_bond_0"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, name, "ports", "1", "counters")
			require.NoError(t, os.MkdirAll(path, 0755))
			require.NoError(t, os.WriteFile(filepath.Join(path, "port_rcv_packets"), []byte("123"), 0644))
			nl := new(MockNetlink)
			nl.On("RdmaResourceList").Return([]*netlink.RdmaResource{{Name: name, RdmaResourceSummaryEntries: map[string]uint64{"qp": 2}}}, nil)
			nl.On("RdmaLinkByName", name).Return(&netlink.RdmaLink{Attrs: netlink.RdmaLinkAttrs{Name: name}}, nil)
			nl.On("RdmaStatistic", mock.Anything).Return((*netlink.RdmaDeviceStatistic)(nil), fmt.Errorf("operation not supported"))
			p := &metricsProbe{netlink: nl, basePath: root}
			values := map[string]float64{}
			require.NoError(t, p.collectOnce(func(name string, _ []string, value float64) { values[name] = value }))
			require.Equal(t, map[string]float64{"qp": 2}, values, "preserve the shared failure behavior for all supported drivers")
			nl.AssertExpectations(t)
		})
	}
}
