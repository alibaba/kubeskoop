package model

import (
	"fmt"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/utils"

	"github.com/samber/lo"
)

type LinkType string

const (
	LinkExternal LinkType = "external"
	LinkInfra    LinkType = "infra"
	LinkVeth     LinkType = "veth"
	LinkIPVlan   LinkType = "ipvlan"
	LinkLocal    LinkType = "local"
)

type LinkAttribute interface {
	GetAttrs() map[string]string
}

type SimpleLinkAttribute struct {
	Interface string
	IP        string
}

func (a SimpleLinkAttribute) GetAttrs() map[string]string {
	return map[string]string{
		"if": a.Interface,
	}
}

type VEthLinkAttribute struct {
	SimpleLinkAttribute
	PeerIndex int
}

func (v VEthLinkAttribute) GetAttrs() map[string]string {
	attrs := v.SimpleLinkAttribute.GetAttrs()
	attrs["peer_id"] = fmt.Sprintf("%d", v.PeerIndex)
	return attrs
}

type NullAttribute struct {
}

func (a NullAttribute) GetAttrs() map[string]string {
	return map[string]string{}
}

type Link struct {
	Type                 LinkType
	Source               *NetNode
	Destination          *NetNode
	Packet               *Packet
	SourceAttribute      LinkAttribute
	DestinationAttribute LinkAttribute

	Level int // for print
}

func (l Link) GetID() string {
	return fmt.Sprintf("%s,%s,%s", l.Type, l.Source.GetID(), l.Destination.GetID())
}

type PacketPath struct {
	origin *NetNode
}

func NewPacketPath(origin *NetNode) *PacketPath {
	return &PacketPath{
		origin: origin,
	}
}

func (p *PacketPath) GetLinkLabel(link *Link, action *Action) string {
	var buf []string
	buf = append(buf, fmt.Sprintf("type=%s", link.Type))
	buf = append(buf, fmt.Sprintf("level=%d", link.Level))
	if action != nil {
		buf = append(buf, fmt.Sprintf("trans=%s", action.Type))
	}

	srcAttrs := map[string]string{}
	if link.SourceAttribute != nil {
		srcAttrs = link.SourceAttribute.GetAttrs()
	}
	dstAttrs := map[string]string{}
	if link.DestinationAttribute != nil {
		dstAttrs = link.DestinationAttribute.GetAttrs()
	}

	if oif, ok := srcAttrs["if"]; ok {
		buf = append(buf, fmt.Sprintf("oif=%s", oif))
	}

	if iif, ok := dstAttrs["if"]; ok {
		buf = append(buf, fmt.Sprintf("iif=%s", iif))
	}

	buf = append(buf,
		fmt.Sprintf("src=%s", link.Packet.Src),
		fmt.Sprintf("dst=%s", link.Packet.Dst),
		fmt.Sprintf("dport=%d", link.Packet.Dport))

	return strings.Join(buf, ",")
}

func (p *PacketPath) GetOriginNode() *NetNode {
	return p.origin
}

func (p *PacketPath) Links() []*Link {
	var (
		links []*Link
	)
	pendingLinks := utils.NewQueue[*Link]()
	addLink := func(level int, action *Action) {
		if action == nil {
			return
		}
		for _, l := range action.Outputs {
			l.Level = level
			pendingLinks.Enqueue(l)
		}
	}

	addLink(0, p.origin.ActionOf(nil))
	for !pendingLinks.Empty() {
		link := pendingLinks.Pop()
		links = append(links, link)
		addLink(link.Level+1, link.Destination.ActionOf(link))
	}
	return links
}

func (p *PacketPath) Paths() string {
	buf := strings.Builder{}
	links := p.Links()
	if len(links) == 0 {
		for _, n := range p.Nodes() {
			buf.WriteString(n.GetID())
		}
		return buf.String()
	}

	for _, l := range links {
		action := l.Destination.ActionOf(l)
		label := p.GetLinkLabel(l, action)

		attr := map[string]string{
			"label": label,
		}
		if action.Type == ActionTypeServe {
			attr["arrowhead"] = "dot"
		}
		attrString := strings.Join(
			lo.MapToSlice(attr, func(k, v string) string { return fmt.Sprintf("%s=%q", k, v) }), ",")

		buf.WriteString(fmt.Sprintf("\"%v\" -> \"%v\" [%s]\n", l.Source.GetID(), l.Destination.GetID(), attrString))
	}
	return buf.String()

}

func (p *PacketPath) Nodes() []*NetNode {
	var (
		nodes []*NetNode
	)
	visited := map[string]struct{}{}

	pendingLinks := utils.NewQueue[*Link]()
	addLink := func(level int, action *Action) {
		if action == nil {
			return
		}
		for _, l := range action.Outputs {
			l.Level = level
			pendingLinks.Enqueue(l)
		}
	}

	addLink(0, p.origin.ActionOf(nil))
	for !pendingLinks.Empty() {
		link := pendingLinks.Pop()
		if _, ok := visited[link.Source.GetID()]; !ok {
			visited[link.Source.GetID()] = struct{}{}
			nodes = append(nodes, link.Source)
		}
		if _, ok := visited[link.Destination.GetID()]; !ok {
			visited[link.Destination.GetID()] = struct{}{}
			nodes = append(nodes, link.Destination)
		}
		addLink(link.Level+1, link.Destination.ActionOf(link))
	}

	if len(nodes) == 0 {
		nodes = append(nodes, p.origin)
	}

	return nodes
}
