package aliyun

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/plugin"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
)

const (
	NetNodeTypeSLB = "slb"
)

type netNodeManager struct {
	infraShim  network.InfraShim
	ipCache    *k8s.IPCache
	processor  service.Processor
	pluginName string
}

func (n *netNodeManager) GetNetNodeFromID(nodeType model.NetNodeType, id string) (model.NetNodeAction, error) {
	switch nodeType {
	case model.NetNodeTypePod, model.NetNodeTypeNode:
		return nil, fmt.Errorf("unspoorted type %s for aliyun net node manager", nodeType)
	case NetNodeTypeSLB:
		return newSLBNode(n.infraShim, n.ipCache, n.processor, n.pluginName, net.ParseIP(id), id)
	default:
		return newExternalNode(n.infraShim, n.ipCache, n.processor, n.pluginName, net.ParseIP(id), id)
	}
}

type externalNode struct {
	ip          net.IP
	netNode     *model.NetNode
	genericNode *plugin.GenericNetNode
	infraShim   network.InfraShim
	ipCache     *k8s.IPCache
	processor   service.Processor
	plugin      string
}

type slbNode struct {
	ip          net.IP
	netNode     *model.NetNode
	genericNode *plugin.GenericNetNode
	infraShim   network.InfraShim
	ipCache     *k8s.IPCache
	processor   service.Processor
	plugin      string
}

func (n *slbNode) Send(_ model.Endpoint, _ model.Protocol) ([]model.Transmission, error) {
	return nil, errors.New("can not send a packet from a loadbalancer node")
}

func (n *slbNode) Receive(upstream *model.Link) ([]model.Transmission, error) {
	upstream.Destination = n.netNode
	pkt := upstream.Packet
	svc, err := n.ipCache.GetServiceFromIP(pkt.Dst.String())
	if err != nil {
		return nil, err
	}

	backends := n.processor.Process(*pkt, svc, nil)
	if len(backends) == 0 {
		m := fmt.Sprintf("service %s/%s has no valid endpoint", svc.Namespace, svc.Name)
		n.netNode.AddSuspicion(model.SuspicionLevelFatal, m)
		n.netNode.DoAction(model.ActionService(nil, []*model.Link{}))
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: n.netNode,
			Err:     errors.New(m),
		}
	}
	if len(backends) > 10 {
		m := fmt.Sprintf("too many backends: %d, stop.", len(backends))
		n.netNode.AddSuspicion(model.SuspicionLevelInfo, m)
		n.netNode.DoAction(model.ActionService(nil, []*model.Link{}))
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: n.netNode,
			Err:     errors.New(m),
		}
	}

	if !lo.ContainsBy(svc.Spec.Ports, func(p v1.ServicePort) bool {
		return p.Port == int32(pkt.Dport) && strings.EqualFold(string(p.Protocol), string(pkt.Protocol))
	}) {
		m := fmt.Sprintf("cannot find port %d protocol %s on service %s/%s",
			pkt.Dport, pkt.Protocol, svc.Namespace, svc.Name)
		n.netNode.AddSuspicion(model.SuspicionLevelFatal, m)
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: n.netNode,
			Err:     errors.New(m),
		}
	}

	nodePort := service.GetNodePort(svc, pkt.Dport, pkt.Protocol)
	targetPort := service.GetTargetPort(svc, pkt.Dport, pkt.Protocol)

	var transmission []model.Transmission
	var lbBackends []network.LoadBalancerBackend
	nodeMap := map[string]struct{}{}
	if svc.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal || n.plugin == ctx.NetworkPluginTerway {
		// terway plugin will add pod ip directly to the loadbalancer backend
		for _, b := range backends {
			t, err := n.ipCache.GetIPType(b.IP)
			if err != nil {
				return nil, err
			}

			nextHop := model.Hop{
				Type: model.NetNodeTypeNode,
			}
			pkt := pkt.DeepCopy()
			pkt.Dst = net.ParseIP(b.IP)
			pkt.Dport = targetPort

			var node *v1.Node
			switch t {
			case model.EndpointTypePod:
				pod, err := n.ipCache.GetPodFromIP(b.IP)
				if err != nil {
					return nil, err
				}
				node, err = n.ipCache.GetNodeFromName(pod.Spec.NodeName)
				if err != nil {
					return nil, err
				}
				nextHop.ID = node.Name
				if n.plugin != ctx.NetworkPluginTerway {
					if _, ok := nodeMap[node.Name]; ok {
						continue
					}
					pkt.Dst = net.ParseIP(node.Status.Addresses[0].Address)
					pkt.Dport = nodePort
					nodeMap[node.Name] = struct{}{}
				}
			case model.EndpointTypeNode:
				node, err = n.ipCache.GetNodeFromIP(b.IP)
				if err != nil {
					return nil, err
				}
				nextHop.ID = node.Name
			}

			trans := model.Transmission{
				NextHop: nextHop,
				Link: &model.Link{
					Type:   model.LinkInfra,
					Source: n.netNode,
					Packet: pkt,
				},
			}
			transmission = append(transmission, trans)

			lbBackend := network.LoadBalancerBackend{
				IP:   pkt.Dst.String(),
				Port: pkt.Dport,
			}

			if n.plugin != ctx.NetworkPluginTerway && node != nil {
				provider := strings.Split(node.Spec.ProviderID, ".")
				if len(provider) == 2 {
					lbBackend.ID = provider[1]
				}
			}
			// todo: support terway plugin eni
			lbBackends = append(lbBackends, lbBackend)
		}
	} else {
		var err error
		transmission, err = n.getTransmissionsToNodePort(nodePort, pkt)
		if err != nil {
			return nil, err
		}
	}

	sus, err := n.infraShim.ExternalToLoadBalancer(svc, pkt, lbBackends)
	if err != nil {
		return nil, err
	}

	n.netNode.Suspicions = append(n.netNode.Suspicions, sus...)
	links := lo.Map(transmission, func(t model.Transmission, _ int) *model.Link {
		return t.Link
	})
	n.netNode.DoAction(model.ActionService(upstream, links))

	return transmission, nil
}

func (n *slbNode) getTransmissionsToNodePort(nodePort uint16, pkt *model.Packet) ([]model.Transmission, error) {
	nodes, err := n.ipCache.GetNodes()
	if err != nil {
		return nil, err
	}

	// todo: exclude nodes with specific labels
	return lo.Map(nodes, func(node *v1.Node, _ int) model.Transmission {
		hop := model.Hop{
			Type: model.NetNodeTypeNode,
			ID:   node.Name,
		}

		ip := node.Status.Addresses[0].Address
		pkt := pkt.DeepCopy()
		pkt.Dst = net.ParseIP(ip)
		pkt.Dport = nodePort

		return model.Transmission{
			NextHop: hop,
			Link: &model.Link{
				Type:   model.LinkInfra,
				Source: n.netNode,
				Packet: pkt,
			},
		}
	}), nil
}

func newExternalNode(infraShim network.InfraShim, ipCache *k8s.IPCache, processor service.Processor, pluginName string, ip net.IP, id string) (model.NetNodeAction, error) {
	netNode := &model.NetNode{
		Type:    model.NetNodeTypeExternal,
		ID:      id,
		Actions: map[*model.Link]*model.Action{},
	}
	return &externalNode{
		ip:          ip,
		ipCache:     ipCache,
		processor:   processor,
		netNode:     netNode,
		genericNode: &plugin.GenericNetNode{NetNode: netNode},
		infraShim:   infraShim,
		plugin:      pluginName,
	}, nil
}

func newSLBNode(infraShim network.InfraShim, ipCache *k8s.IPCache, processor service.Processor, pluginName string, ip net.IP, id string) (model.NetNodeAction, error) {
	netNode := &model.NetNode{
		Type:    NetNodeTypeSLB,
		ID:      id,
		Actions: map[*model.Link]*model.Action{},
	}
	return &slbNode{
		ip:          ip,
		ipCache:     ipCache,
		processor:   processor,
		netNode:     netNode,
		genericNode: &plugin.GenericNetNode{NetNode: netNode},
		infraShim:   infraShim,
		plugin:      pluginName,
	}, nil
}

func (n *externalNode) Send(dst model.Endpoint, protocol model.Protocol) ([]model.Transmission, error) {
	t, err := n.ipCache.GetIPType(dst.IP)
	if err != nil {
		return nil, err
	}

	switch t {
	case model.EndpointTypePod:
		pod, err := n.ipCache.GetPodFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		node, err := n.ipCache.GetNodeFromName(pod.Spec.NodeName)
		if err != nil {
			return nil, err
		}
		return n.sendToNode(dst, protocol, node)
	case model.EndpointTypeNode:
		node, err := n.ipCache.GetNodeFromIP(dst.IP)
		if err != nil {
			return nil, err
		}
		return n.sendToNode(dst, protocol, node)
	case model.EndpointTypeLoadbalancer:
		return n.sendToLoadBalancer(dst, protocol)
	case model.EndpointTypeExternal, model.EndpointTypeService:
		msg := fmt.Sprintf("cannot send packet from external ip to %s ip %s", t, dst.IP)
		n.netNode.AddSuspicion(model.SuspicionLevelFatal, msg)
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: n.netNode,
			Err:     errors.New(msg),
		}
	default:
		msg := fmt.Sprintf("not supported endpoint type %s", t)
		n.netNode.AddSuspicion(model.SuspicionLevelFatal, msg)
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: n.netNode,
			Err:     errors.New(msg),
		}
	}
}

func (n *externalNode) sendToNode(dst model.Endpoint, protocol model.Protocol, node *v1.Node) ([]model.Transmission, error) {
	pkt := &model.Packet{
		Src:      n.ip,
		Dst:      net.ParseIP(dst.IP),
		Dport:    dst.Port,
		Protocol: protocol,
	}

	sus, err := n.infraShim.ExternalToNode(node, pkt)
	if err != nil {
		return nil, err
	}
	n.netNode.Suspicions = append(n.netNode.Suspicions, sus...)
	trans := model.Transmission{
		NextHop: model.Hop{
			Type: model.NetNodeTypeNode,
			ID:   node.Name,
		},
		Link: &model.Link{
			Type:   model.LinkInfra,
			Source: n.netNode,
			Packet: pkt,
		},
	}

	action := model.ActionSend([]*model.Link{trans.Link})
	n.netNode.DoAction(action)

	return []model.Transmission{trans}, nil
}

func (n *externalNode) sendToLoadBalancer(dst model.Endpoint, protocol model.Protocol) ([]model.Transmission, error) {
	pkt := &model.Packet{
		Src:      n.ip,
		Dst:      net.ParseIP(dst.IP),
		Dport:    dst.Port,
		Protocol: protocol,
	}

	hop := model.Hop{
		Type: NetNodeTypeSLB,
		ID:   dst.IP,
	}

	trans := model.Transmission{
		NextHop: hop,
		Link: &model.Link{
			Type:   model.LinkExternal,
			Source: n.netNode,
			Packet: pkt,
		},
	}

	n.netNode.DoAction(model.ActionSend([]*model.Link{trans.Link}))
	return []model.Transmission{trans}, nil
}

func (n *externalNode) Receive(upstream *model.Link) ([]model.Transmission, error) {
	return n.genericNode.Receive(upstream)
}
