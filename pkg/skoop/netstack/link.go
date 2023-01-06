package netstack

import (
	"net"
)

type Addr struct {
	*net.IPNet
}

type Neigh struct {
	Family       int
	LinkIndex    int
	State        int
	Type         int
	Flags        int
	IP           net.IP
	HardwareAddr net.HardwareAddr
}

const (
	LinkUP = iota + 1
	LinkDown
	LinkUnknown
)

const (
	LinkDriverVeth = "veth"
	LinkDriverIPIP = "ipip"
)

var (
	defaultIP = net.IPv4(169, 254, 0, 1)
)

type Interface struct {
	Name        string            `json:"name"`
	Index       int               `json:"index"`
	MTU         int               `json:"MTU"`
	Driver      string            `json:"driver"`
	Addrs       []Addr            `json:"addrs"`
	State       int               `json:"state"`
	DevSysctls  map[string]string `json:"dev_sysctls"`
	NeighInfo   []Neigh           `json:"neigh_info"`
	FdbInfo     []Neigh           `json:"fdb_info"`
	PeerIndex   int               `json:"peer_index"`
	MasterIndex int               `json:"master_index"`
}

func GetDefaultIPv4(iface *Interface) (net.IP, net.IPMask) {
	for _, addr := range iface.Addrs {
		if addr.IP.To4() != nil {
			return addr.IP.To4(), addr.Mask
		}
	}
	return defaultIP, net.CIDRMask(32, 32)
}
