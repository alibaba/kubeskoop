package rdma

import (
	"strconv"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const linkTypeTroce = "troce"

// Preserve the driver's counter names and raw units. In particular, do not
// assume Mellanox's data-counter units or congestion counters for this driver.
var troceCounters = []string{
	"port_rcv_data", "port_xmit_data", "port_rcv_packets", "port_xmit_packets",
	"port_rcv_errors", "port_xmit_discards", "port_rcv_constraint_errors",
	"port_xmit_constraint_errors", "port_rcv_remote_physical_errors",
	"port_rcv_switch_relay_errors", "local_link_integrity_errors",
	"excessive_buffer_overrun_errors", "link_downed", "link_error_recovery",
	"multicast_rcv_packets", "multicast_xmit_packets", "unicast_rcv_packets",
	"unicast_xmit_packets", "port_xmit_wait", "symbol_error", "VL15_dropped",
	"packet_seq_err", "out_of_buffer", "out_of_seq", "local_ack_timeout_err",
	"rnr_nak_retry_err", "implied_nak_seq_err", "req_cqe_err", "resp_cqe_err",
	"duplicate_request",
}

var troceMetrics = func() []probe.SingleMetricsOpts {
	opts := make([]probe.SingleMetricsOpts, 0, len(troceCounters))
	for _, name := range troceCounters {
		opts = append(opts, probe.SingleMetricsOpts{
			Name:           linkTypeTroce + "_" + name,
			VariableLabels: rdmaDevPortLabels,
			Help:           "SNIC driver counter " + name + " in raw driver units.",
			ValueType:      prometheus.CounterValue,
		})
	}
	return opts
}()

// collectTroce keeps driver-specific filtering and fallback out of the shared collector.
func (p *metricsProbe) collectTroce(link *netlink.RdmaLink, labels []string, emit probe.Emit) {
	seen := make(map[string]bool)
	emitCounter := func(port uint32, key string, value uint64) {
		known := false
		for _, name := range troceCounters {
			if key == name {
				known = true
				break
			}
		}
		series := strconv.FormatUint(uint64(port), 10) + "/" + key
		if !known || seen[series] {
			return
		}
		seen[series] = true
		portLabels := append(append([]string{}, labels...), strconv.FormatUint(uint64(port), 10))
		emit(linkTypeTroce+"_"+key, portLabels, float64(value))
	}
	statistics, err := p.netlink.RdmaStatistic(link)
	if err != nil {
		log.Errorf("failed get SNIC statistics %s: %v", link.Attrs.Name, err)
	} else if statistics != nil {
		for _, port := range statistics.RdmaPortStatistics {
			for key, value := range port.Statistics {
				emitCounter(port.PortIndex, key, value)
			}
		}
	}
	for _, directory := range []string{"hw_counters", "counters"} {
		ports, err := metricsFromSysFSDirectory(link, p.basePath, directory)
		if err != nil {
			log.Warnf("failed get SNIC %s %s: %v", directory, link.Attrs.Name, err)
			continue
		}
		for _, port := range ports {
			for key, value := range port.Statistics {
				emitCounter(port.PortIndex, key, value)
			}
		}
	}
}
