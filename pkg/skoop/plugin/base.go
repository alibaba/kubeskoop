package plugin

import (
	"net"

	"github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	k8s2 "github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"

	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	v1 "k8s.io/api/core/v1"
)

type Plugin interface {
	CreatePod(pod *k8s2.Pod) (model2.NetNodeAction, error)
	CreateNode(node *k8s2.NodeInfo) (model2.NetNodeAction, error)
}

type transmissionFunc func(pkt *model2.Packet, iif string) (model2.Transmission, error)

type SimplePluginNode interface {
	ToPod(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, pod *v1.Pod) ([]model2.Transmission, error)
	ToHost(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, node *v1.Node) ([]model2.Transmission, error)
	ToService(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, service *v1.Service) ([]model2.Transmission, error)
	ToExternal(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol) ([]model2.Transmission, error)
	Serve(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol) ([]model2.Transmission, error)
}

type BasePluginNode struct {
	*model2.NetNode
	IPCache          *k8s2.IPCache
	SimplePluginNode SimplePluginNode
}

func (b *BasePluginNode) Send(dst model2.Endpoint, protocol model2.Protocol) (trans []model2.Transmission, err error) {
	ipType, err := b.IPCache.GetIPType(dst.IP)
	if err != nil {
		return nil, err
	}

	switch ipType {
	case model2.EndpointTypePod:
		pod, err := b.IPCache.GetPodFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		return b.SimplePluginNode.ToPod(nil, dst, protocol, pod)
	case model2.EndpointTypeNode:
		host, err := b.IPCache.GetNodeFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		return b.SimplePluginNode.ToHost(nil, dst, protocol, host)
	case model2.EndpointTypeService:
		svc, err := b.IPCache.GetServiceFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		return b.SimplePluginNode.ToService(nil, dst, protocol, svc)
	default:
		return b.SimplePluginNode.ToExternal(nil, dst, protocol)
	}
}

func (b *BasePluginNode) Receive(upstream *model2.Link) (trans []model2.Transmission, err error) {
	upstream.Destination = b.NetNode

	dstIP := upstream.Packet.Dst.String()
	dstType, err := b.IPCache.GetIPType(dstIP)
	if err != nil {
		return nil, err
	}

	dst := model2.Endpoint{IP: dstIP, Port: upstream.Packet.Dport, Type: dstType}
	protocol := upstream.Packet.Protocol

	switch dstType {
	case model2.EndpointTypePod:
		pod, err := b.IPCache.GetPodFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		return b.SimplePluginNode.ToPod(upstream, dst, protocol, pod)
	case model2.EndpointTypeNode:
		host, err := b.IPCache.GetNodeFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		if host.Name == b.ID {
			//to myself
			svc, err := b.IPCache.GetServiceFromNodePort(dst.Port, protocol)
			if err != nil {
				return nil, err
			}
			if svc != nil {
				return b.SimplePluginNode.ToService(upstream, dst, protocol, svc)
			}
			return b.SimplePluginNode.Serve(upstream, dst, protocol)
		}
		return b.SimplePluginNode.ToHost(upstream, dst, protocol, host)
	case model2.EndpointTypeService:
		svc, err := b.IPCache.GetServiceFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		return b.SimplePluginNode.ToService(upstream, dst, protocol, svc)
	default:
		return b.SimplePluginNode.ToExternal(upstream, dst, protocol)
	}
}

var _ model2.NetNodeAction = &BasePluginNode{}

type route struct {
	routes map[string]assertions.RouteAssertion
}

func newRoute(routes map[string]assertions.RouteAssertion) *route {
	if routes != nil {
		return &route{routes: routes}
	}
	return &route{routes: make(map[string]assertions.RouteAssertion)}
}

func (r *route) AddRoute(cidr string, dev string, gateway *net.IP, scope netstack.Scope) error {
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	r.routes[cidr] = assertions.RouteAssertion{
		Dev:   &dev,
		Scope: &scope,
		Gw:    gateway,
	}
	return nil
}

func (r *route) Assert(netAssertion *assertions.NetstackAssertion, pkt *model2.Packet) error {
	cidrs := lo.MapToSlice(r.routes, func(k string, _ assertions.RouteAssertion) *net.IPNet {
		_, n, _ := net.ParseCIDR(k)
		return n
	})

	matchedCIDR := smallestMatchingCIDR(pkt.Dst, cidrs)
	if matchedCIDR == nil {
		return nil
	}

	return netAssertion.AssertRoute(r.routes[matchedCIDR.String()], *pkt, "", "")
}

func smallestMatchingCIDR(ip net.IP, cidr []*net.IPNet) *net.IPNet {
	matched := lo.Filter(cidr, func(c *net.IPNet, _ int) bool { return c.Contains(ip) })
	if len(matched) == 0 {
		return nil
	}

	slices.SortFunc(matched, func(a, b *net.IPNet) bool {
		onesA, _ := a.Mask.Size()
		onesB, _ := b.Mask.Size()
		return onesA > onesB
	})

	return matched[0]
}

func ack(pkt *model2.Packet) *model2.Packet {
	return &model2.Packet{
		Src:      pkt.Dst,
		Sport:    pkt.Dport,
		Dst:      pkt.Src,
		Dport:    pkt.Sport,
		Protocol: pkt.Protocol,
	}
}
