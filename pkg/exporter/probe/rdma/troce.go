package rdma

import (
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
)

const linkTypeTroce = "troce"

// Preserve native driver names and units. Standard port counter descriptions
// follow the RDMA sysfs ABI; hardware counters retain driver-specific scope.
var troceCounters = map[string]string{
	// Standard RDMA port counters.
	"port_rcv_data":                   "Received data in 4-byte words, including protocol overhead; multiply the rate by four for bytes/second.",
	"port_xmit_data":                  "Transmitted data in 4-byte words, including protocol overhead; multiply the rate by four for bytes/second.",
	"port_rcv_packets":                "Packets received by the RDMA port.",
	"port_xmit_packets":               "Packets transmitted by the RDMA port.",
	"port_rcv_errors":                 "Received packets with errors.",
	"port_xmit_discards":              "Outbound packets discarded by the port.",
	"port_rcv_constraint_errors":      "Received packets rejected by port constraint checks.",
	"port_xmit_constraint_errors":     "Outbound packets rejected by port constraint checks.",
	"port_rcv_remote_physical_errors": "Received packets indicating a remote physical error.",
	"port_rcv_switch_relay_errors":    "Received packets discarded by switch forwarding.",
	"local_link_integrity_errors":     "Local link integrity threshold violations.",
	"excessive_buffer_overrun_errors": "Input buffer overrun events.",
	"link_downed":                     "Link-down events following failed error recovery.",
	"link_error_recovery":             "Successful link error recovery events.",
	"multicast_rcv_packets":           "Multicast packets received by the port.",
	"multicast_xmit_packets":          "Multicast packets transmitted by the port.",
	"unicast_rcv_packets":             "Unicast packets received by the port.",
	"unicast_xmit_packets":            "Unicast packets transmitted by the port.",
	"port_xmit_wait":                  "Transmit-wait ticks; the driver tick duration is unverified, so this value is not seconds.",
	"symbol_error":                    "Physical link symbol errors.",
	"VL15_dropped":                    "Virtual lane 15 packets dropped for insufficient resources.",

	// Native SNIC hardware counters.
	"packet_seq_err":        "Driver-reported packet sequence errors; the exact triggering conditions are not verified.",
	"out_of_buffer":         "Receive buffer shortages; observed when a receive work queue has no available entries.",
	"out_of_seq":            "Out-of-sequence packets detected by the receiver.",
	"local_ack_timeout_err": "Local acknowledgement timer expirations.",
	"rnr_nak_retry_err":     "Receiver-not-ready (RNR) negative acknowledgements associated with retries; not necessarily retry-limit exhaustion.",
	"implied_nak_seq_err":   "Driver-reported implied negative-acknowledgement sequence errors; the exact triggering conditions are not verified.",
	"req_cqe_err":           "Requester completion queue entries (CQEs) with errors.",
	"resp_cqe_err":          "Responder completion queue entries (CQEs) with errors.",
	"duplicate_request":     "Duplicate RDMA requests detected by the receiver.",
}

var troceMetrics = func() []probe.SingleMetricsOpts {
	opts := make([]probe.SingleMetricsOpts, 0, len(troceCounters))
	for name, help := range troceCounters {
		opts = append(opts, probe.SingleMetricsOpts{
			Name:           linkTypeTroce + "_" + name,
			VariableLabels: rdmaDevPortLabels,
			Help:           help,
			ValueType:      prometheus.CounterValue,
		})
	}
	return opts
}()
