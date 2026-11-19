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
	opts := append(append([]probe.SingleMetricsOpts{}, troceMetrics...), troceNetdevMetrics...)
	registry := prometheus.NewPedanticRegistry()
	collector := probe.NewBatchMetrics(probe.BatchMetricsOpts{
		Namespace: probe.MetricsNamespace, Subsystem: probeName,
		VariableLabels: probe.StandardMetricsLabels, SingleMetricsOpts: opts,
	}, func(emit probe.Emit) error {
		labels := make([]string, len(probe.StandardMetricsLabels))
		labels = append(labels, "troce_bond_0", "troce")
		emit("troce_out_of_seq", append(append([]string{}, labels...), "1"), 3)
		emitTroceNetdev(map[string]uint64{"mac_rx_pfc_pri3": 4, "rx_bytes": 5}, append(labels, "bond0", "reth0"), emit)
		return nil
	})
	require.NoError(t, registry.Register(collector))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 3, "SNIC fields must have valid descriptors and labels")
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

func TestTrocePFCAllowlist(t *testing.T) {
	values := map[string]float64{}
	emitTroceNetdev(map[string]uint64{
		"mac_rx_pfc_pri3": 12, "mac_rx_pfc_pri3_duration": 99,
		"mac_rx_pfc_pri3_transition": 2, "tx_bytes": 1000,
		"mac_tx_bytes": 1200, "mac_rx_fcs_e": 3,
		"tx_queue_0_bytes": 500, "mac_tx_pfc_pri8": 10,
	}, []string{"node", "", "", "troce_bond_0", "troce", "bond0", "reth0"}, func(name string, labels []string, value float64) {
		values[name] = value
		if name == "troce_pfc_packets" {
			require.Equal(t, []string{"rx", "3"}, labels[len(labels)-2:])
		}
	})
	require.Equal(t, map[string]float64{"troce_pfc_packets": 12, "troce_pfc_duration_raw": 99, "troce_pfc_transitions": 2,
		"troce_netdev_tx_bytes": 1000, "troce_netdev_mac_tx_bytes": 1200, "troce_netdev_mac_rx_fcs_e": 3}, values)
}

func TestTroceBondMapping(t *testing.T) {
	root := t.TempDir()
	ib := filepath.Join(root, "infiniband")
	net := filepath.Join(root, "net")
	path := filepath.Join(ib, "troce_bond_0", "ports", "1", "gid_attrs", "ndevs")
	require.NoError(t, os.MkdirAll(path, 0755))
	for _, index := range []string{"0", "1"} {
		require.NoError(t, os.WriteFile(filepath.Join(path, index), []byte("bond7\n"), 0644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(net, "bond7", "bonding"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(net, "bond7", "bonding", "slaves"), []byte("reth2 reth9\n"), 0644))
	mapping, err := troceNetdevs(ib, net, "troce_bond_0")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"bond7": {"reth2", "reth9"}}, mapping)
	// A failed GID entry must not discard the valid mappings from this port.
	require.NoError(t, os.Mkdir(filepath.Join(path, "2"), 0755))
	mapping, err = troceNetdevs(ib, net, "troce_bond_0")
	require.Error(t, err)
	require.Equal(t, map[string][]string{"bond7": {"reth2", "reth9"}}, mapping)
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
