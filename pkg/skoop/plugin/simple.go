package plugin

import (
	"fmt"
	"net"

	assertions2 "github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	k8s2 "github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	netstack2 "github.com/alibaba/kubeskoop/pkg/skoop/netstack"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"
	"github.com/samber/lo"
)

type simpleVEthPod struct {
	netNode *model2.NetNode
	podInfo *k8s2.Pod
	net     *assertions2.NetstackAssertion
	k8s     *assertions2.KubernetesAssertion
	mtu     int
	iface   string
	ipCache *k8s2.IPCache
}

func newSimpleVEthPod(pod *k8s2.Pod, ipCache *k8s2.IPCache, mtu int, iface string) (*simpleVEthPod, error) {
	netNode := model2.NewNetNode(fmt.Sprintf("%s/%s", pod.Namespace, pod.PodName), model2.NetNodeTypePod)
	return &simpleVEthPod{
		netNode: netNode,
		podInfo: pod,
		mtu:     mtu,
		iface:   iface,
		net:     assertions2.NewNetstackAssertion(netNode, &pod.NetNS),
		k8s:     assertions2.NewKubernetesAssertion(netNode),
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
	p.net.AssertNetDevice("eth0", netstack2.Interface{
		MTU:   p.mtu,
		State: netstack2.LinkUP,
	})
	p.net.AssertNetDevice("lo", netstack2.Interface{
		State: netstack2.LinkUP,
	})
	return nil
}

func (p *simpleVEthPod) Send(dst model2.Endpoint, protocol model2.Protocol) ([]model2.Transmission, error) {
	err := p.assert()
	if err != nil {
		return nil, err
	}

	pkt := &model2.Packet{
		Dst:      net.ParseIP(dst.IP),
		Dport:    dst.Port,
		Protocol: protocol,
	}

	addr, _, err := p.podInfo.NetNS.Router.RouteSrc(pkt, "", "")
	if err != nil {
		if err == netstack2.ErrNoRouteToHost {
			p.netNode.AddSuspicion(model2.SuspicionLevelFatal, fmt.Sprintf("no route to host: %v", dst))
		}
		return nil, &assertions2.CannotBuildTransmissionError{
			SrcNode: p.netNode,
			Err:     fmt.Errorf("no route to host: %v", err)}
	}
	pkt.Src = net.ParseIP(addr)

	iface, _ := lo.Find(p.podInfo.NetNS.Interfaces, func(i netstack2.Interface) bool { return i.Name == "eth0" })
	link := &model2.Link{
		Type:   model2.LinkVeth,
		Source: p.netNode,
		Packet: pkt,
		SourceAttribute: model2.VEthLinkAttribute{
			SimpleLinkAttribute: model2.SimpleLinkAttribute{
				Interface: "eth0",
				IP:        addr,
			},
			PeerIndex: iface.PeerIndex,
		},
	}
	err = p.net.AssertRoute(assertions2.RouteAssertion{Dev: utils.ToPointer("eth0")}, *pkt, "", "")
	if err != nil {
		return nil, err
	}

	p.netNode.DoAction(model2.ActionSend([]*model2.Link{link}))

	return []model2.Transmission{
		{
			NextHop: model2.Hop{
				Type: model2.NetNodeTypeNode,
				ID:   p.podInfo.NodeName,
			},
			Link: link,
		},
	}, nil
}

func (p *simpleVEthPod) Receive(upstream *model2.Link) ([]model2.Transmission, error) {
	if upstream.Type != model2.LinkVeth {
		return nil, fmt.Errorf("unexpect upstream type to receive, expect veth, but: %v", upstream.Type)
	}
	upstream.Destination = p.netNode
	upstream.DestinationAttribute = model2.SimpleLinkAttribute{
		Interface: "eth0",
	}
	pkt := upstream.Packet
	err := p.assert()
	if err != nil {
		return nil, err
	}
	err = p.net.AssertRoute(assertions2.RouteAssertion{Dev: utils.ToPointer("eth0")}, *ack(pkt), "", "")
	if err != nil {
		return nil, err
	}

	p.net.AssertListen(pkt.Dst, pkt.Dport, pkt.Protocol)
	p.netNode.DoAction(model2.ActionServe(upstream))
	return []model2.Transmission{}, nil
}
