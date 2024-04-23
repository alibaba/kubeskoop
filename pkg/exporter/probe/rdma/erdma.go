package rdma

import (
	"strings"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
)

const (
	linkTypeERdma = "erdma"
)

var (
	erdmaStatisticCounterEntries = map[string]string{
		"accept_failed_cnt":           "The total number of failed connection accept attempts.",
		"accept_success_cnt":          "The total number of successful connection accept attempts.",
		"accept_total_cnt":            "The total number of connection accept attempts, successful or not.",
		"cmdq_comp_cnt":               "The total number of command queue completions processed.",
		"cmdq_cq_armed_cnt":           "The total number of command queue completion events that have been armed.",
		"cmdq_eq_event_cnt":           "The total number of command queue event queue events received.",
		"cmdq_eq_notify_cnt":          "The total number of command queue event queue notifications triggered.",
		"cmdq_submitted_cnt":          "The total number of command queue submissions.",
		"connect_failed_cnt":          "The total number of failed connection attempts.",
		"connect_reset_cnt":           "The total number of connection attempts that have been reset.",
		"connect_success_cnt":         "The total number of successful connection attempts.",
		"connect_timeout_cnt":         "The total number of connection attempts that timed out.",
		"connect_total_cnt":           "The total number of connection attempts, successful or not.",
		"erdma_aeq_event_cnt":         "The total number of ERDMA asynchronous event queue events received.",
		"erdma_aeq_notify_cnt":        "The total number of ERDMA asynchronous event queue notifications triggered.",
		"hw_bps_limit_drop_cnt":       "The total number of packets dropped due to hardware bandwidth limit.",
		"hw_disable_drop_cnt":         "The total number of packets dropped due to hardware being disabled.",
		"hw_pps_limit_drop_cnt":       "The total number of packets dropped due to hardware packets-per-second limit.",
		"hw_rx_bps_limit_drop_cnt":    "The total number of received packets dropped due to hardware receive bandwidth limit.",
		"hw_rx_bytes_cnt":             "The total number of bytes received by the hardware.",
		"hw_rx_disable_drop_cnt":      "The total number of received packets dropped due to receive hardware being disabled.",
		"hw_rx_packets_cnt":           "The total number of packets received by the hardware.",
		"hw_rx_pps_limit_drop_cnt":    "The total number of received packets dropped due to hardware receive packets-per-second limit.",
		"hw_tx_bytes_cnt":             "The total number of bytes transmitted by the hardware.",
		"hw_tx_packets_cnt":           "The total number of packets transmitted by the hardware.",
		"hw_tx_reqs_cnt":              "The total number of transmit requests processed by the hardware.",
		"listen_create_cnt":           "The total number of successfully created listen sockets.",
		"listen_destroy_cnt":          "The total number of destroyed listen sockets.",
		"listen_failed_cnt":           "The total number of failed attempts to create listen sockets.",
		"listen_ipv6_cnt":             "The total number of listen sockets created for IPv6 addresses.",
		"listen_success_cnt":          "The total number of successful listen operations.",
		"reject_cnt":                  "The total number of received connection requests that were rejected.",
		"reject_failed_cnt":           "The total number of failed attempts to reject connection requests.",
		"verbs_alloc_mr_cnt":          "The total number of successful memory region allocations using verbs API.",
		"verbs_alloc_mr_failed_cnt":   "The total number of failed memory region allocation attempts using verbs API.",
		"verbs_alloc_pd_cnt":          "The total number of successful protection domain allocations using verbs API.",
		"verbs_alloc_pd_failed_cnt":   "The total number of failed protection domain allocation attempts using verbs API.",
		"verbs_alloc_uctx_cnt":        "The total number of successful user context allocations using verbs API.",
		"verbs_alloc_uctx_failed_cnt": "The total number of failed user context allocation attempts using verbs API.",
		"verbs_create_cq_cnt":         "The total number of successful completion queue creations using verbs API.",
		"verbs_create_cq_failed_cnt":  "The total number of failed completion queue creation attempts using verbs API.",
		"verbs_destroy_cq_failed_cnt": "The total number of failed completion queue deletion using verbs API.",
		"verbs_create_qp_cnt":         "The total number of successful queue pair creations using verbs API.",
		"verbs_create_qp_failed_cnt":  "The total number of failed queue pair creation attempts using verbs API.",
		"verbs_destroy_qp_cnt":        "The total number of failed queue pair deletion using verbs API.",
		"verbs_dealloc_pd_cnt":        "The total number of deallocated protection domains using verbs API.",
		"verbs_dealloc_uctx_cnt":      "The total number of deallocated user contexts using verbs API.",
		"verbs_dereg_mr_cnt":          "The total number of successful memory region deregistrations using verbs API.",
		"verbs_dereg_mr_failed_cnt":   "The total number of failed memory region deregistration attempts using verbs API.",
		"verbs_destroy_cq_cnt":        "The total number of destroyed completion queues using verbs API.",
		"verbs_destroy_qp_failed_cnt": "The total number of failed attempts to destroy queue pairs (QPs) using verbs API.",
		"verbs_get_dma_mr_cnt":        "The total number of successful direct memory access (DMA) memory region acquisitions using verbs API.",
		"verbs_get_dma_mr_failed_cnt": "The total number of failed attempts to acquire direct memory access (DMA) memory regions using verbs API.",
		"verbs_reg_usr_mr_cnt":        "The total number of user memory regions successfully registered with the verbs API.",
		"verbs_reg_usr_mr_failed_cnt": "The total number of failed attempts to register user memory regions with the verbs API.",
	}
	erdmaMetrics = lo.Map(lo.Keys(erdmaStatisticCounterEntries), func(k string, _ int) probe.SingleMetricsOpts {
		return probe.SingleMetricsOpts{
			Name:           strings.Join([]string{linkTypeERdma, k}, "_"),
			VariableLabels: rdmaDevPortLabels,
			Help:           erdmaStatisticCounterEntries[k],
			ValueType:      prometheus.CounterValue,
		}
	})
)
