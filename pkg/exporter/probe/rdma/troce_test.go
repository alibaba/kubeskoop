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
		labels = append(labels, "troce_bond_0", "troce")
		emit("troce_out_of_seq", append(append([]string{}, labels...), "1"), 3)
		return nil
	})
	require.NoError(t, registry.Register(collector))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1, "SNIC fields must have valid descriptors and labels")
}

func TestTroceSysfsSurvivesNetlinkFailure(t *testing.T) {
	root := t.TempDir()
	name := "troce_bond_0"
	path := filepath.Join(root, name, "ports", "1", "counters")
	require.NoError(t, os.MkdirAll(path, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "port_rcv_packets"), []byte("123\n"), 0644))
	nl := new(MockNetlink)
	nl.On("RdmaResourceList").Return([]*netlink.RdmaResource{{Name: name, RdmaResourceSummaryEntries: map[string]uint64{"qp": 2}}}, nil)
	nl.On("RdmaLinkByName", name).Return(&netlink.RdmaLink{Attrs: netlink.RdmaLinkAttrs{Name: name}}, nil)
	nl.On("RdmaStatistic", mock.Anything).Return((*netlink.RdmaDeviceStatistic)(nil), fmt.Errorf("operation not supported"))
	p := &metricsProbe{netlink: nl, basePath: root}
	values := map[string]float64{}
	require.NoError(t, p.collectOnce(func(name string, labels []string, value float64) {
		values[name] = value
		require.Contains(t, labels, linkTypeTroce)
	}))
	require.Equal(t, float64(123), values["troce_port_rcv_packets"])
	require.Equal(t, float64(2), values["qp"])
	nl.AssertExpectations(t)
}

func TestTroceHardwareFallbackAndPrecedence(t *testing.T) {
	root := t.TempDir()
	name := "troce_bond_0"
	for _, directory := range []string{"counters", "hw_counters"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, name, "ports", "1", directory), 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, name, "ports", "1", "counters", "port_rcv_data"), []byte("34226688"), 0644))
	for key, value := range map[string]string{"out_of_seq": "9", "req_cqe_err": "7", "lifespan": "10"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name, "ports", "1", "hw_counters", key), []byte(value), 0644))
	}
	nl := new(MockNetlink)
	nl.On("RdmaResourceList").Return([]*netlink.RdmaResource{{Name: name}}, nil)
	nl.On("RdmaLinkByName", name).Return(&netlink.RdmaLink{Attrs: netlink.RdmaLinkAttrs{Name: name}}, nil)
	nl.On("RdmaStatistic", mock.Anything).Return(&netlink.RdmaDeviceStatistic{RdmaPortStatistics: []*netlink.RdmaPortStatistic{{PortIndex: 1, Statistics: map[string]uint64{"out_of_seq": 12}}}}, nil)
	p := &metricsProbe{netlink: nl, basePath: root}
	values := map[string][]float64{}
	require.NoError(t, p.collectOnce(func(name string, _ []string, value float64) { values[name] = append(values[name], value) }))
	require.Equal(t, []float64{12}, values["troce_out_of_seq"])
	require.Equal(t, []float64{7}, values["troce_req_cqe_err"])
	require.Equal(t, []float64{34226688}, values["troce_port_rcv_data"], "preserve the driver's 4-byte-word unit")
	for metric := range values {
		require.Contains(t, metric, "troce_", "SNIC statistics must not emit mlx5 or shared aliases")
	}
	require.NotContains(t, values, "troce_lifespan")
	nl.AssertExpectations(t)
}
