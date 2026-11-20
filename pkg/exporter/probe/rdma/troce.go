package rdma

import (
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
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
