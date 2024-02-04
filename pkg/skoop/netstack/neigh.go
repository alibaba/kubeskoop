package netstack

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// Neighbor Cache Entry States.
const (
	NudNone       = 0x00
	NudIncomplete = 0x01
	NudReachable  = 0x02
	NudStale      = 0x04
	NudDelay      = 0x08
	NudProbe      = 0x10
	NudFailed     = 0x20
	NudNoarp      = 0x40
	NudPermanent  = 0x80
)

type NeighResult struct {
	State  int
	LLAddr net.HardwareAddr
	// for vxlan usage
	DST net.IP
}

type Neighbour struct {
	interfaces []Interface
}

func NewNeigh(interfaces []Interface) *Neighbour {
	return &Neighbour{
		interfaces: interfaces,
	}
}

func (n *Neighbour) ProbeNeigh(ip net.IP, linkIndex int) (*NeighResult, error) {
	var in *Interface
	for _, inf := range n.interfaces {
		if inf.Index == linkIndex {
			in = &inf
			break
		}
	}
	if in == nil {
		return nil, fmt.Errorf("cannot found link: %v", linkIndex)
	}
	var (
		matchNeigh *Neigh
	)
	result := &NeighResult{}
	for _, neigh := range in.NeighInfo {
		if neigh.LinkIndex == linkIndex && neigh.IP.Equal(ip) {
			matchNeigh = &neigh
			break
		}
	}
	// no cache for neigh
	if matchNeigh == nil {
		return nil, nil
	}
	result.LLAddr = matchNeigh.HardwareAddr
	result.State = matchNeigh.State

	if result.State == NudPermanent {
		for _, fdb := range in.FdbInfo {
			if fdb.HardwareAddr.String() == matchNeigh.HardwareAddr.String() {
				result.DST = fdb.IP
			}
		}
	}
	return result, nil
}

func (n *Neighbour) ProbeRouteNeigh(route *Route, dst net.IP) (*Neigh, error) {
	for _, ni := range n.interfaces {
		if ni.Name == route.OifName {
			for _, neigh := range ni.NeighInfo {
				if neigh.Family == unix.AF_INET &&
					(route.Gw == nil && neigh.IP.Equal(dst)) ||
					(route.Gw != nil && neigh.IP.Equal(route.Gw)) {
					return &neigh, nil
				}
			}
		}
	}
	return nil, nil
}
