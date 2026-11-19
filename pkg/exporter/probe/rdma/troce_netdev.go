package rdma

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/safchain/ethtool"
)

var troceNetdevCounters = []string{"tx_bytes", "rx_bytes", "tx_dropped", "rx_errors", "mac_tx_pause_ctrl", "mac_rx_pause_ctrl",
	"mac_rx_bytes", "mac_tx_bytes", "mac_rx_packets", "mac_tx_packets", "mac_rx_fcs_e"}

var troceNetdevMetrics = func() []probe.SingleMetricsOpts {
	var opts []probe.SingleMetricsOpts
	for _, name := range troceNetdevCounters {
		opts = append(opts, probe.SingleMetricsOpts{Name: "troce_netdev_" + name, Help: "SNIC interface driver counter " + name + "; includes non-RDMA traffic.", VariableLabels: []string{"device", "type", "bond", "netdev"}, ValueType: prometheus.CounterValue})
	}
	for _, name := range []string{"pfc_packets", "pfc_duration_raw", "pfc_transitions"} {
		opts = append(opts, probe.SingleMetricsOpts{Name: "troce_" + name, Help: "SNIC physical interface " + name + "; duration retains driver units.", VariableLabels: []string{"device", "type", "bond", "netdev", "direction", "priority"}, ValueType: prometheus.CounterValue})
	}
	return opts
}()

func (p *metricsProbe) collectTroceNetdev(device string, labels []string, emit probe.Emit) error {
	root := p.basePath
	if root == "" {
		root = ibDefaultBasePath
	}
	mapping, mappingErr := troceNetdevs(root, "/sys/class/net", device)
	if len(mapping) == 0 {
		return mappingErr
	}
	client, err := ethtool.NewEthtool()
	if err != nil {
		return err
	}
	defer client.Close()
	firstErr := mappingErr
	for bond, slaves := range mapping {
		for _, slave := range slaves {
			stats, err := client.Stats(slave)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", slave, err)
				}
				continue
			}
			emitTroceNetdev(stats, append(append([]string{}, labels...), bond, slave), emit)
		}
	}
	return firstErr
}

func emitTroceNetdev(stats map[string]uint64, labels []string, emit probe.Emit) {
	for _, name := range troceNetdevCounters {
		if value, ok := stats[name]; ok {
			emit("troce_netdev_"+name, labels, float64(value))
		}
	}
	for _, direction := range []string{"rx", "tx"} {
		for priority := 0; priority < 8; priority++ {
			prefix := "mac_" + direction + "_pfc_pri" + strconv.Itoa(priority)
			for _, field := range []struct{ suffix, metric string }{{"", "pfc_packets"}, {"_duration", "pfc_duration_raw"}, {"_transition", "pfc_transitions"}} {
				if value, ok := stats[prefix+field.suffix]; ok {
					emit("troce_"+field.metric, append(append([]string{}, labels...), direction, strconv.Itoa(priority)), float64(value))
				}
			}
		}
	}
}

// Discover the actual RDMA netdev and its slaves rather than inferring their
// names from a device index. A disappeared interface is retried next scrape.
func troceNetdevs(ibRoot, netRoot, device string) (map[string][]string, error) {
	entries, err := os.ReadDir(filepath.Join(ibRoot, device, "ports"))
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	var firstErr error
	for _, port := range entries {
		names, err := os.ReadDir(filepath.Join(ibRoot, device, "ports", port.Name(), "gid_attrs", "ndevs"))
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("read netdev mapping: %w", err)
			}
			continue
		}
		for _, entry := range names {
			data, err := os.ReadFile(filepath.Join(ibRoot, device, "ports", port.Name(), "gid_attrs", "ndevs", entry.Name()))
			// Unpopulated GID table entries exist in sysfs but return EINVAL.
			if errors.Is(err, syscall.EINVAL) || os.IsNotExist(err) {
				continue
			}
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			name := strings.TrimSpace(string(data))
			if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
				continue
			}
			if _, exists := result[name]; exists {
				continue
			}
			slaves, err := os.ReadFile(filepath.Join(netRoot, name, "bonding", "slaves"))
			if os.IsNotExist(err) {
				result[name] = []string{name}
			} else if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			} else {
				result[name] = strings.Fields(string(slaves))
			}
		}
	}
	return result, firstErr
}
