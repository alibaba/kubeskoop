package plugin

import (
	"fmt"
	"net"

	"github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"
	"github.com/samber/lo"
)

type simpleVEthPod struct {
	netNode *model.NetNode
	podInfo *k8s.Pod
	net     *assertions.NetstackAssertion
	k8s     *assertions.KubernetesAssertion
	mtu     int
	iface   string
	ipCache *k8s.IPCache
}

func newSimpleVEthPod(pod *k8s.Pod, ipCache *k8s.IPCache, mtu int, iface string) (*simpleVEthPod, error) {
	netNode := model.NewNetNode(fmt.Sprintf("%s/%s", pod.Namespace, pod.PodName), model.NetNodeTypePod)
	return &simpleVEthPod{
		netNode: netNode,
		podInfo: pod,
		mtu:     mtu,
		iface:   iface,
		net:     assertions.NewNetstackAssertion(netNode, &pod.NetNS),
		k8s:     assertions.NewKubernetesAssertion(netNode),
		ipCache: ipCache,
	}, nil
}

func (p *simpleVEthPod) assert() error {
	pod, err := p.ipCache.GetPodFromName(p.podInfo.Namespace, p.podInfo.PodName)
	if err != nil {
		return err
	}
	p.k8s.AssertPod(pod)
	p.net.AssertDefaultRule()
	p.net.AssertNoPolicyRoute()
	p.net.AssertNoIPTables()
	p.net.AssertDefaultAccept()
	p.net.AssertNetDevice("eth0", netstack.Interface{
		MTU:   p.mtu,
		State: netstack.LinkUP,
	})
	p.net.AssertNetDevice("lo", netstack.Interface{
		State: netstack.LinkUP,
	})
	return nil
}

func (p *simpleVEthPod) Send(dst model.Endpoint, protocol model.Protocol) ([]model.Transmission, error) {
	err := p.assert()
	if err != nil {
		return nil, err
	}

	pkt := &model.Packet{
		Dst:      net.ParseIP(dst.IP),
		Dport:    dst.Port,
		Protocol: protocol,
	}

	addr, dstRoute, err := p.podInfo.NetNS.Router.RouteSrc(pkt, "", "")
	if err != nil {
		if err == netstack.ErrNoRouteToHost {
			p.netNode.AddSuspicion(model.SuspicionLevelFatal, fmt.Sprintf("no route to host: %v", dst))
		}
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: p.netNode,
			Err:     fmt.Errorf("no route to host: %v", err)}
	}
	neigh, err := p.podInfo.NetNS.Neighbour.ProbeRouteNeigh(dstRoute, pkt.Dst)
	if err != nil {
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: p.netNode,
			Err:     fmt.Errorf("pod neigh system probe failed: %v", err),
		}
	}
	if neigh != nil && (neigh.State == netstack.NudFailed || neigh.State == netstack.NudIncomplete) {
		if dstRoute.Gw == nil {
			p.netNode.AddSuspicion(model.SuspicionLevelCritical, fmt.Sprintf("dst: %v ARP resolve failed.", pkt.Dst.String()))
		} else {
			p.netNode.AddSuspicion(model.SuspicionLevelCritical, fmt.Sprintf("dst: %v route's gateway: %v ARP resolve failed.", pkt.Dst.String(), dstRoute.Gw.String()))
		}
	}

	pkt.Src = net.ParseIP(addr)

	iface, _ := lo.Find(p.podInfo.NetNS.Interfaces, func(i netstack.Interface) bool { return i.Name == "eth0" })
	link := &model.Link{
		Type:   model.LinkVeth,
		Source: p.netNode,
		Packet: pkt,
		SourceAttribute: model.VEthLinkAttribute{
			SimpleLinkAttribute: model.SimpleLinkAttribute{
				Interface: "eth0",
				IP:        addr,
			},
			PeerIndex: iface.PeerIndex,
		},
	}
	err = p.net.AssertRoute(assertions.RouteAssertion{Dev: utils.ToPointer("eth0")}, *pkt, "", "")
	if err != nil {
		return nil, err
	}

	p.netNode.DoAction(model.ActionSend([]*model.Link{link}))

	return []model.Transmission{
		{
			NextHop: model.Hop{
				Type: model.NetNodeTypeNode,
				ID:   p.podInfo.NodeName,
			},
			Link: link,
		},
	}, nil
}

func (p *simpleVEthPod) Receive(upstream *model.Link) ([]model.Transmission, error) {
	if upstream.Type != model.LinkVeth {
		return nil, fmt.Errorf("unexpect upstream type to receive, expect veth, but: %v", upstream.Type)
	}
	upstream.Destination = p.netNode
	upstream.DestinationAttribute = model.SimpleLinkAttribute{
		Interface: "eth0",
	}
	pkt := upstream.Packet
	err := p.assert()
	if err != nil {
		return nil, err
	}
	err = p.net.AssertRoute(assertions.RouteAssertion{Dev: utils.ToPointer("eth0")}, *ack(pkt), "", "")
	if err != nil {
		return nil, err
	}

	p.net.AssertListen(pkt.Dst, pkt.Dport, pkt.Protocol)
	p.netNode.DoAction(model.ActionServe(upstream))
	return []model.Transmission{}, nil
}

type GenericNetNode struct {
	NetNode *model.NetNode
}

func (n *GenericNetNode) Send(_ model.Endpoint, _ model.Protocol) ([]model.Transmission, error) {
	n.NetNode.AddSuspicion(model.SuspicionLevelFatal, "non pod/node address as source is not supported")
	return nil, &assertions.CannotBuildTransmissionError{
		SrcNode: n.NetNode,
		Err:     fmt.Errorf("non pod/node address as source is not supported"),
	}
}

func (n *GenericNetNode) Receive(upstream *model.Link) ([]model.Transmission, error) {
	upstream.Destination = n.NetNode
	n.NetNode.DoAction(model.ActionServe(upstream))
	return nil, nil
}
