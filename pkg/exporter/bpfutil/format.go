package bpfutil

import (
	"fmt"
	"net"
	"strings"
	"syscall"
)

// GetAddrStr get string format ip address,default in ipv4
func GetAddrStr(proto uint16, addr [16]byte) string {
	switch proto {
	case syscall.ETH_P_IPV6:
		return fmt.Sprintf("[%s]", net.IP(addr[:]).String())
	default:
		return net.IP(addr[:4]).String()
	}
}

/*
	enum {
		TCP_ESTABLISHED = 1,
		TCP_SYN_SENT,
		TCP_SYN_RECV,
		TCP_FIN_WAIT1,
		TCP_FIN_WAIT2,
		TCP_TIME_WAIT,
		TCP_CLOSE,
		TCP_CLOSE_WAIT,
		TCP_LAST_ACK,
		TCP_LISTEN,
		TCP_CLOSING,
		TCP_NEW_SYN_RECV,

		TCP_MAX_STATES
	};
*/
func GetSkcStateStr(state uint8) string {
	switch state {
	case 1:
		return "TCP_ESTABLISHED"
	case 2:
		return "TCP_SYN_SENT"
	case 3:
		return "TCP_SYN_RECV"
	case 10:
		return "TCP_LISTEN"
	default:
		return "TCP_OTHER"
	}
}

// GetProtoStr get proto sting, default IP
func GetProtoStr(proto uint8) string {
	switch proto {
	case syscall.IPPROTO_TCP:
		return "TCP"
	case syscall.IPPROTO_UDP:
		return "UDP"
	case syscall.IPPROTO_ICMP:
		return "ICMP"
	case syscall.IPPROTO_ICMPV6:
		return "ICMP6"
	default:
		return "IP"
	}
}

func GetHumanTimes(ns uint64) string {
	if ns > 1000*1000 {
		return fmt.Sprintf("%d ms", ns/(1000*1000))
	} else if ns > 1000 {
		return fmt.Sprintf("%d us", ns/1000)
	}

	return fmt.Sprintf("%d ns", ns)
}

func GetCommString(comm [20]int8) string {
	buf := make([]byte, 20)
	for idx := range comm {
		buf[idx] = byte(comm[idx])
	}

	return strings.TrimSpace(string(buf))
}

func GetTCPState(no uint8) string {

	return "UNKNOW"
}
