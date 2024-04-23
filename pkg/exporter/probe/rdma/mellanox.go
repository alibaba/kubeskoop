package rdma

import (
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
)

const (
	linkTypeMellanox = "mellanox_mlx5"
)

var (
	mlx5 = map[string]string{
		"rx_write_requests":          "The number of received WRITE requests for the associated QPs.",
		"rx_read_requests":           "The number of received READ requests for the associated QPs.",
		"rx_atomic_requests":         "The number of received ATOMIC request for the associated QPs.",
		"out_of_buffer":              "The number of drops occurred due to lack of WQE for the associated QPs.",
		"out_of_sequence":            "The number of out of sequence packets received.",
		"duplicate_request":          "Number of duplicate request packets.",
		"rnr_nak_retry_err":          "The number of received RNR NAK packets. The QP retry limit was not exceeded.",
		"packet_seq_err":             "The number of received NAK sequence error packets. The QP retry limit was not exceeded.",
		"implied_nak_seq_err":        "Number of time the requested decided an ACK with a PSN larger than the expected PSN for an RDMA read or response.",
		"local_ack_timeout_err":      "The number of times QP's ack timer expired for RC, XRC, DCT QPs at the sender side.",
		"rx_dct_connect":             "The number of received connection request for the associated DCTs.",
		"resp_local_length_error":    "The number of times responder detected local length errors.",
		"resp_cqe_error":             "The number of times responder detected CQEs completed with errors.",
		"req_cqe_error":              "The number of times requester detected CQEs completed with errors.",
		"req_remote_invalid_request": "The number of times requester detected remote invalid request errors.",
		"req_remote_access_errors":   "The number of times requester detected remote access errors.",
		"resp_remote_access_errors":  "The number of times responder detected remote access errors.",
		"resp_cqe_flush_error":       "The number of times responder detected CQEs completed with flushed errors.",
		"req_cqe_flush_error":        "The number of times requester detected CQEs completed with flushed errors.",
		"roce_adp_retrans":           "The number of adaptive retransmissions for RoCE traffic",
		"roce_adp_retrans_to":        "The number of times RoCE traffic reached timeout due to adaptive retransmission",
		"roce_slow_restart":          "The number of times RoCE slow restart was used",
		"roce_slow_restart_cnps":     "The number of times RoCE slow restart generated CNP packets",
		"roce_slow_restart_trans":    "The number of times RoCE slow restart changed state to slow restart",
		"rp_cnp_ignored":             "The number of CNP packets received and ignored by the Reaction Point HCA.",
		"rp_cnp_handled":             "The number of CNP packets handled by the Reaction Point HCA to throttle the transmission rate.",
		"np_ecn_marked_roce_packets": "The number of RoCEv2 packets received by the notification point which were marked for experiencing the congestion (ECN bits where '11' on the ingress RoCE traffic) .",
		"np_cnp_sent":                "The number of CNP packets sent by the Notification Point when it noticed congestion experienced in the RoCEv2 IP header (ECN bits).",
		"rx_icrc_encapsulated":       "The number of RoCE packets with ICRC errors.",
	}
	mlx5Metrics = lo.Map(lo.Keys(mlx5), func(k string, _ int) probe.SingleMetricsOpts {
		return probe.SingleMetricsOpts{
			Name:           strings.Join([]string{linkTypeMellanox, k}, "_"),
			VariableLabels: rdmaDevPortLabels,
			Help:           mlx5[k],
			ValueType:      prometheus.CounterValue,
		}
	})
)
