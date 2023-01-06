package model

import (
	"fmt"
	"net"

	"golang.org/x/exp/slices"
)

type Packet struct {
	Src      net.IP
	Sport    uint16
	Dst      net.IP
	Dport    uint16
	Protocol Protocol
	Encap    *Packet
	Mark     uint32
}

func (p *Packet) String() string {
	return fmt.Sprintf("%s %s:%d->%s:%d [%d]", p.Protocol, p.Src, p.Sport, p.Dst, p.Dport, p.Mark)
}

func (p *Packet) DeepCopy() *Packet {
	pkt := &Packet{
		Src:      slices.Clone(p.Src),
		Sport:    p.Sport,
		Dst:      slices.Clone(p.Dst),
		Dport:    p.Dport,
		Protocol: p.Protocol,
		Encap:    nil,
		Mark:     p.Mark,
	}

	if p.Encap != nil {
		pkt.Encap = p.Encap.DeepCopy()
	}

	return pkt
}
