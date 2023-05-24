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
	Name        string            `json:"n"`
	Index       int               `json:"i"`
	MTU         int               `json:"m"`
	Driver      string            `json:"d"`
	Addrs       []Addr            `json:"a"`
	State       int               `json:"st"`
	DevSysctls  map[string]string `json:"s"`
	NeighInfo   []Neigh           `json:"ne"`
	FdbInfo     []Neigh           `json:"f"`
	PeerIndex   int               `json:"p"`
	MasterIndex int               `json:"mi"`
}

func GetDefaultIPv4(iface *Interface) (net.IP, net.IPMask) {
	for _, addr := range iface.Addrs {
		if addr.IP.To4() != nil {
			return addr.IP.To4(), addr.Mask
		}
	}
	return defaultIP, net.CIDRMask(32, 32)
}
