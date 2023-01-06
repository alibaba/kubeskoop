package plugin

import (
	"fmt"
	"net"
	"strings"

	assertions2 "github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	k8s2 "github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	netstack2 "github.com/alibaba/kubeskoop/pkg/skoop/netstack"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"

	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
)

const (
	flannelVxlanInterface = "flannel.1"
)

type FlannelBackendType string

const (
	FlannelBackendTypeHostGW FlannelBackendType = "host-gw"
	FlannelBackendTypeVxlan  FlannelBackendType = "vxlan"
	FlannelBackendTypeAlloc  FlannelBackendType = "alloc"
)

type FlannelPluginOptions struct {
	InfraShim        network.InfraShim
	Bridge           string
	Interface        string
	PodMTU           int
	IPMasq           bool
	ClusterCIDR      *net.IPNet
	CNIMode          FlannelBackendType
	ServiceProcessor service.Processor
}

type flannelPlugin struct {
	hostOptions      *flannelHostOptions
	serviceProcessor service.Processor
	podMTU           int

	infraShim network.InfraShim
	ipCache   *k8s2.IPCache
}

func (f *flannelPlugin) CreatePod(pod *k8s2.Pod) (model2.NetNodeAction, error) {
	return newSimpleVEthPod(pod, f.ipCache, f.podMTU, "eth0")
}

func (f *flannelPlugin) CreateNode(node *k8s2.NodeInfo) (model2.NetNodeAction, error) {
	flannelHost, err := newFlannelHost(f.ipCache, node, f.infraShim, f.serviceProcessor, f.hostOptions)
	if err != nil {
		return nil, err
	}
	return &BasePluginNode{
		NetNode:          flannelHost.netNode,
		IPCache:          f.ipCache,
		SimplePluginNode: flannelHost,
	}, nil
}

func NewFlannelPlugin(ctx *ctx.Context, options *FlannelPluginOptions) (Plugin, error) {
	return &flannelPlugin{
		podMTU: options.PodMTU,
		hostOptions: &flannelHostOptions{
			Bridge:      options.Bridge,
			ClusterCIDR: options.ClusterCIDR,
			Interface:   options.Interface,
			CNIMode:     options.CNIMode,
			IPMasq:      options.IPMasq,
		},
		serviceProcessor: options.ServiceProcessor,
		infraShim:        options.InfraShim,
		ipCache:          ctx.ClusterConfig().IPCache,
	}, nil
}

type flannelNodeInfo struct {
	Vtep        net.IP
	CIDR        *net.IPNet
	BackendType FlannelBackendType
	NodeIP      net.IP
	Dev         *netstack2.Interface
	Route       assertions2.RouteAssertion
}

type flannelRoute struct {
	*route
	localNetAssertion *assertions2.NetstackAssertion
	localPodCIDR      *net.IPNet
	clusterCIDR       *net.IPNet
	localVTEP         net.IP
	ipCache           *k8s2.IPCache
	nodeInfoCache     map[string]*flannelNodeInfo
}

func newFlannelRoute(parentRoute map[string]assertions2.RouteAssertion, localPodCIDR *net.IPNet,
	clusterCIDR *net.IPNet, localNetAssertion *assertions2.NetstackAssertion, localNode *v1.Node, ipCache *k8s2.IPCache) *flannelRoute {
	route := &flannelRoute{
		route:             newRoute(parentRoute),
		localNetAssertion: localNetAssertion,
		localPodCIDR:      localPodCIDR,
		clusterCIDR:       clusterCIDR,
		localVTEP:         net.ParseIP(localNode.Annotations["flannel.alpha.coreos.com/public-ip"]),
		ipCache:           ipCache,
		nodeInfoCache:     map[string]*flannelNodeInfo{},
	}

	return route
}

func (r *flannelRoute) AssertBackend(pkt *model2.Packet) error {
	hostInfo, err := r.getDstInfo(pkt)
	if err != nil {
		return err
	}

	if hostInfo != nil && hostInfo.Dev != nil {
		r.localNetAssertion.AssertNetDevice(hostInfo.Dev.Name, netstack2.Interface{
			State: netstack2.LinkUP,
			MTU:   hostInfo.Dev.MTU,
		})
		r.localNetAssertion.AssertSysctls(map[string]string{
			fmt.Sprintf("net.ipv4.conf.%s.forwarding",
				strings.Replace(hostInfo.Dev.Name, ".", "/", -1)): "1",
		}, model2.SuspicionLevelFatal)
		if hostInfo.BackendType == "vxlan" {
			err = r.localNetAssertion.AssertVxlanVtep(hostInfo.Vtep, hostInfo.NodeIP, flannelVxlanInterface)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *flannelRoute) Encap(opkt *model2.Packet) (*model2.Packet, error) {
	pkt := opkt
	hostInfo, err := r.getDstInfo(opkt)
	if err != nil {
		return nil, err
	}
	if hostInfo == nil {
		return pkt, nil
	}

	if hostInfo.BackendType == "vxlan" {
		pkt = &model2.Packet{
			Src:      r.localVTEP,
			Sport:    pkt.Sport,
			Dst:      hostInfo.NodeIP,
			Dport:    8472,
			Protocol: model2.UDP,
			Encap:    opkt.DeepCopy(),
		}
	}

	return pkt, nil
}

func (r *flannelRoute) Decap(opkt *model2.Packet) (*model2.Packet, error) {
	if opkt.Encap != nil {
		if !opkt.Dst.Equal(r.localVTEP) {
			return nil, fmt.Errorf("encap dst %s not match local vtep %s", opkt.Dst, r.localVTEP)
		}
		return opkt.Encap, nil
	}
	return opkt, nil
}

func (r *flannelRoute) getDstInfo(pkt *model2.Packet) (*flannelNodeInfo, error) {
	pod, err := r.ipCache.GetPodFromIP(pkt.Dst.String())
	if err != nil {
		return nil, err
	}
	if pod == nil {
		return nil, nil
	}
	return r.getDstNodeInfo(pod.Spec.NodeName)
}

func (r *flannelRoute) getDstNodeInfo(nodeName string) (*flannelNodeInfo, error) {
	if info, ok := r.nodeInfoCache[nodeName]; ok {
		return info, nil
	}

	node, err := r.ipCache.GetNodeFromName(nodeName)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("node %s not found in k8s nodes", nodeName)
	}

	ip, cidr, err := net.ParseCIDR(node.Spec.PodCIDR)
	if err != nil {
		return nil, err
	}
	nextNodeIP := node.Annotations["flannel.alpha.coreos.com/public-ip"]
	if nextNodeIP == "" {
		return nil, fmt.Errorf("node %s does not have flannel public-ip annotation", nodeName)
	}
	backendTypeAnnotation, ok := node.Annotations["flannel.alpha.coreos.com/backend-type"]
	if !ok {
		return nil, fmt.Errorf("node %s does not have flannel backend-type annotation", nodeName)
	}

	var backendType FlannelBackendType
	var dev *netstack2.Interface
	var vtep net.IP
	if strings.EqualFold(backendTypeAnnotation, string(FlannelBackendTypeVxlan)) {
		vtep = ip
		dev = &netstack2.Interface{
			Name:   flannelVxlanInterface,
			MTU:    1450,
			Driver: "vxlan",
		}
		backendType = FlannelBackendTypeVxlan
	} else if strings.EqualFold(backendTypeAnnotation, string(FlannelBackendTypeHostGW)) {
		vtep = net.ParseIP(nextNodeIP)
		backendType = FlannelBackendTypeHostGW
	} else {
		backendType = FlannelBackendType(backendTypeAnnotation)
	}

	route := assertions2.RouteAssertion{}
	if vtep != nil && !vtep.IsUnspecified() {
		route.Gw = &vtep
		if dev != nil {
			route.Dev = &dev.Name
		}
	}

	info := &flannelNodeInfo{
		Vtep:        vtep,
		CIDR:        cidr,
		BackendType: backendType,
		NodeIP:      net.ParseIP(nextNodeIP),
		Dev:         dev,
		Route:       route,
	}

	r.nodeInfoCache[nodeName] = info
	return info, nil
}

func (r *flannelRoute) Assert(netAssertion *assertions2.NetstackAssertion, pkt *model2.Packet) error {
	if (r.localPodCIDR != nil && r.localPodCIDR.Contains(pkt.Dst)) ||
		!r.clusterCIDR.Contains(pkt.Dst) {
		return r.route.Assert(netAssertion, pkt)
	}
	nodeInfo, err := r.getDstInfo(pkt)
	if err != nil {
		return err
	}
	if nodeInfo == nil {
		return nil
	}
	return netAssertion.AssertRoute(nodeInfo.Route, *pkt, "", "")
}

type flannelHost struct {
	nodeInfo         *k8s2.NodeInfo
	netNode          *model2.NetNode
	ipCache          *k8s2.IPCache
	infraShim        network.InfraShim
	serviceProcessor service.Processor

	bridge      string
	clusterCIDR *net.IPNet
	podCIDR     *net.IPNet
	iface       string
	cniMode     FlannelBackendType
	ipMasq      bool
	gateway     net.IP

	net   *assertions2.NetstackAssertion
	k8s   *assertions2.KubernetesAssertion
	route *flannelRoute
}

type flannelHostOptions struct {
	Bridge      string
	ClusterCIDR *net.IPNet
	Interface   string
	CNIMode     FlannelBackendType
	IPMasq      bool
	Gateway     net.IP
}

func newFlannelHost(ipCache *k8s2.IPCache, nodeInfo *k8s2.NodeInfo, infraShim network.InfraShim,
	serviceProcessor service.Processor, options *flannelHostOptions) (*flannelHost, error) {
	//if serviceProcessor == nil {
	//	return nil, fmt.Errorf("service processor cannot be nil")
	//}

	k8sNode, err := ipCache.GetNodeFromName(nodeInfo.NodeName)
	if err != nil {
		return nil, err
	}

	_, podCIDR, err := net.ParseCIDR(k8sNode.Spec.PodCIDR)
	if err != nil {
		return nil, err
	}

	netNode := model2.NewNetNode(nodeInfo.NodeName, model2.NetNodeTypeNode)

	assertion := assertions2.NewNetstackAssertion(netNode, &nodeInfo.NetNS)
	k8sAssertion := assertions2.NewKubernetesAssertion(netNode)

	host := &flannelHost{
		netNode:          netNode,
		nodeInfo:         nodeInfo,
		ipCache:          ipCache,
		infraShim:        infraShim,
		podCIDR:          podCIDR,
		net:              assertion,
		k8s:              k8sAssertion,
		bridge:           options.Bridge,
		clusterCIDR:      options.ClusterCIDR,
		iface:            options.Interface,
		cniMode:          options.CNIMode,
		ipMasq:           options.IPMasq,
		gateway:          options.Gateway,
		serviceProcessor: serviceProcessor,
	}

	err = host.initRoute()
	if err != nil {
		return nil, err
	}

	err = host.basicCheck()
	if err != nil {
		return nil, err
	}

	return host, nil
}

var _ SimplePluginNode = &flannelHost{}

func (h *flannelHost) transmissionToPod(pkt *model2.Packet, pod *v1.Pod, iif string) (model2.Transmission, error) {
	if !pod.Spec.HostNetwork && pod.Spec.NodeName == h.nodeInfo.NodeName {
		// check local veth
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		if peerNetNS, ok := lo.Find(h.nodeInfo.SubNetNSInfo, func(info netstack2.NetNSInfo) bool { return info.Key == key }); ok {
			h.net.AssertVEthPeerBridge("eth0", &peerNetNS, "cni0")
		}

		// send to local pod
		err := h.checkRoute(pkt)
		if err != nil {
			return model2.Transmission{}, err
		}

		pktOut := pkt.DeepCopy()
		err = h.masquerade(pktOut)
		if err != nil {
			return model2.Transmission{}, err
		}

		link := &model2.Link{
			Type:            model2.LinkVeth,
			Source:          h.netNode,
			Packet:          pktOut,
			SourceAttribute: model2.SimpleLinkAttribute{Interface: h.bridge},
		}

		nextHop := model2.Hop{
			Type: model2.NetNodeTypePod,
			ID:   pkt.Dst.String(),
		}

		return model2.Transmission{
			NextHop: nextHop,
			Link:    link,
		}, nil
	}

	node, err := h.ipCache.GetNodeFromName(pod.Spec.NodeName)
	if err != nil {
		return model2.Transmission{}, err
	}

	return h.transmissionToNode(pkt, node, iif)
}

func (h *flannelHost) transmissionToNode(pkt *model2.Packet, node *v1.Node, iif string) (model2.Transmission, error) {
	pktOut := pkt.DeepCopy()

	err := h.masquerade(pktOut)
	if err != nil {
		return model2.Transmission{}, err
	}

	err = h.checkRoute(pkt)
	if err != nil {
		return model2.Transmission{}, err
	}

	err = h.route.AssertBackend(pktOut)
	if err != nil {
		return model2.Transmission{}, err
	}

	out, err := h.route.Encap(pktOut)
	if err != nil {
		return model2.Transmission{}, err
	}

	localNode, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return model2.Transmission{}, err
	}

	if h.infraShim != nil {
		suspicions, err := h.infraShim.NodeToNode(localNode, h.iface, node, pktOut)
		if err != nil {
			return model2.Transmission{}, err
		}
		h.netNode.Suspicions = append(h.netNode.Suspicions, suspicions...)
	}

	link := &model2.Link{
		Type:            model2.LinkInfra,
		Source:          h.netNode,
		Packet:          out,
		SourceAttribute: model2.SimpleLinkAttribute{Interface: h.iface},
	}
	nextHop := model2.Hop{
		Type: model2.NetNodeTypeNode,
		ID:   node.Name,
	}

	return model2.Transmission{
		NextHop: nextHop,
		Link:    link,
	}, nil
}

func (h *flannelHost) transmissionToExternal(pkt *model2.Packet, iif string) (model2.Transmission, error) {
	pktOut := pkt.DeepCopy()
	err := h.masquerade(pktOut)
	if err != nil {
		return model2.Transmission{}, err
	}
	err = h.checkRoute(pktOut)
	if err != nil {
		return model2.Transmission{}, err
	}

	node, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return model2.Transmission{}, err
	}
	if h.infraShim != nil {
		suspicions, err := h.infraShim.NodeToExternal(node, h.iface, pktOut)
		if err != nil {
			return model2.Transmission{}, err
		}
		h.netNode.Suspicions = append(h.netNode.Suspicions, suspicions...)
	}

	link := &model2.Link{
		Type:            model2.LinkInfra,
		Source:          h.netNode,
		Packet:          pktOut,
		SourceAttribute: model2.SimpleLinkAttribute{Interface: h.iface},
	}
	nextHop := model2.Hop{
		Type: model2.NetNodeTypeExternal,
		ID:   pktOut.Dst.String(),
	}
	return model2.Transmission{
		NextHop: nextHop,
		Link:    link,
	}, nil
}
func (h *flannelHost) ToPod(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, pod *v1.Pod) ([]model2.Transmission, error) {
	makeLink := func(pkt *model2.Packet, iif string) (model2.Transmission, error) {
		return h.transmissionToPod(pkt, pod, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *flannelHost) ToHost(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, node *v1.Node) ([]model2.Transmission, error) {
	makeLink := func(pkt *model2.Packet, iif string) (model2.Transmission, error) {
		return h.transmissionToNode(pkt, node, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *flannelHost) ToService(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, service *v1.Service) ([]model2.Transmission, error) {
	iif := h.detectIif(upstream)
	var pkt *model2.Packet
	if upstream != nil {
		upstream.DestinationAttribute = model2.SimpleLinkAttribute{Interface: iif}
		pkt = upstream.Packet
	} else {
		pkt = &model2.Packet{
			Dst:      net.ParseIP(dst.IP),
			Dport:    dst.Port,
			Protocol: protocol,
		}
		src, _, err := h.nodeInfo.Router.RouteSrc(pkt, "", "")
		if err != nil {
			if err == netstack2.ErrNoRouteToHost {
				h.netNode.AddSuspicion(model2.SuspicionLevelFatal, fmt.Sprintf("no route to host: %v", dst))
				return nil, &assertions2.CannotBuildTransmissionError{
					SrcNode: h.netNode,
					Err:     fmt.Errorf("no route to host: %v", dst),
				}
			}
			return nil, err
		}
		pkt.Src = net.ParseIP(src)
	}

	backends := h.serviceProcessor.Process(*pkt, service)
	if len(backends) == 0 {
		h.netNode.Suspicions = append(h.netNode.Suspicions, model2.Suspicion{
			Level:   model2.SuspicionLevelFatal,
			Message: fmt.Sprintf("service %s/%s has no valid endpoint", service.Namespace, service.Name),
		})
		return nil, &assertions2.CannotBuildTransmissionError{
			SrcNode: h.netNode,
			Err:     fmt.Errorf("service %s/%s has no valid endpoint", service.Namespace, service.Name),
		}
	}

	if err := h.serviceProcessor.Validate(*pkt, backends, h.nodeInfo.NetNS); err != nil {
		h.netNode.Suspicions = append(h.netNode.Suspicions, model2.Suspicion{
			Level:   model2.SuspicionLevelFatal,
			Message: fmt.Sprintf("validate endpoint of service %s/%s failed: %s", service.Namespace, service.Name, err),
		})
	}

	var nfAssertion func(pktIn model2.Packet, pktOut []model2.Packet, iif string)
	if upstream != nil {
		nfAssertion = h.net.AssertNetfilterForward
	} else {
		nfAssertion = h.net.AssertNetfilterSend
	}

	var transmissions []model2.Transmission
	for _, backend := range backends {
		pktOut := &model2.Packet{
			Src:      pkt.Src,
			Sport:    pkt.Sport,
			Dst:      net.ParseIP(backend.IP),
			Dport:    backend.Port,
			Protocol: protocol,
		}

		if backend.Masquerade {
			ip, _, err := h.nodeInfo.Router.RouteSrc(pktOut, "", "")
			if err != nil {
				return nil, err
			}
			pktOut.Src = net.ParseIP(ip)
		}

		backendType, err := h.ipCache.GetIPType(backend.IP)
		if err != nil {
			return nil, err
		}
		switch backendType {
		case model2.EndpointTypePod:
			pod, err := h.ipCache.GetPodFromIP(backend.IP)
			if err != nil {
				return nil, err
			}
			if pod == nil {
				return nil, fmt.Errorf("cannot find pod from ip %s", backend.IP)
			}
			transmission, err := h.transmissionToPod(pktOut, pod, iif)
			if err != nil {
				return nil, err
			}
			transmissions = append(transmissions, transmission)
		case model2.EndpointTypeNode:
			node, err := h.ipCache.GetNodeFromIP(backend.IP)
			if err != nil {
				return nil, err
			}
			if node == nil {
				return nil, fmt.Errorf("cannot find node from ip %s", backend.IP)
			}
			transmission, err := h.transmissionToNode(pktOut, node, iif)
			if err != nil {
				return nil, err
			}
			transmissions = append(transmissions, transmission)
		default:
			transmission, err := h.transmissionToExternal(pktOut, iif)
			if err != nil {
				return nil, err
			}
			transmissions = append(transmissions, transmission)
		}
	}

	links := lo.Map(transmissions, func(t model2.Transmission, _ int) *model2.Link { return t.Link })
	h.netNode.DoAction(model2.ActionService(upstream, links))

	pktOutList := lo.Map(links, func(l *model2.Link, _ int) model2.Packet { return *l.Packet })
	nfAssertion(*pkt, pktOutList, iif)
	return transmissions, nil
}

func (h *flannelHost) ToExternal(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol) ([]model2.Transmission, error) {
	makeLink := func(pkt *model2.Packet, iif string) (model2.Transmission, error) {
		return h.transmissionToExternal(pkt, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *flannelHost) to(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, transmit transmissionFunc) ([]model2.Transmission, error) {
	iif := h.detectIif(upstream)
	var action *model2.Action
	var transmission model2.Transmission
	var pkt *model2.Packet
	var nfAssertionFunc func(pktIn model2.Packet, pktOut []model2.Packet, iif string)

	if upstream != nil {
		if upstream.Type == model2.LinkVeth {
			// send from local pod, assert veth
			if attr, ok := upstream.SourceAttribute.(model2.VEthLinkAttribute); ok {
				h.net.AssertVEthOnBridge(attr.PeerIndex, h.bridge)
			}
		}

		upstream.DestinationAttribute = model2.SimpleLinkAttribute{Interface: iif}
		var err error
		pkt, err = h.route.Decap(upstream.Packet)
		if err != nil {
			return nil, err
		}
		transmission, err = transmit(pkt, iif)
		if err != nil {
			return nil, err
		}
		if transmission.Link == nil {
			return nil, nil
		}
		nfAssertionFunc = h.net.AssertNetfilterForward
		action = model2.ActionForward(upstream, []*model2.Link{transmission.Link})
	} else {
		pkt = &model2.Packet{
			Dst:      net.ParseIP(dst.IP),
			Dport:    dst.Port,
			Protocol: protocol,
		}
		addr, _, err := h.nodeInfo.Router.RouteSrc(pkt, "", "")
		if err != nil {
			return nil, err
		}

		pkt.Src = net.ParseIP(addr)
		transmission, err = transmit(pkt, iif)
		if err != nil {
			return nil, err
		}
		if transmission.Link == nil {
			return nil, nil
		}
		nfAssertionFunc = h.net.AssertNetfilterSend
		action = model2.ActionSend([]*model2.Link{transmission.Link})
	}

	pktOut := transmission.Link.Packet
	nfAssertionFunc(*pkt, []model2.Packet{*pktOut}, iif)

	h.netNode.DoAction(action)
	return []model2.Transmission{transmission}, nil
}

func (h *flannelHost) Serve(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol) ([]model2.Transmission, error) {
	iif := h.detectIif(upstream)
	if upstream.Packet.Encap != nil {
		innerPacket, err := h.route.Decap(upstream.Packet)
		if err != nil {
			return nil, err
		}
		pod, err := h.ipCache.GetPodFromIP(innerPacket.Dst.String())
		if err != nil {
			return nil, err
		}
		if pod == nil {
			return nil, fmt.Errorf("inner packet pod %s not found", innerPacket.Dst)
		}
		return h.ToPod(upstream, model2.Endpoint{
			IP:   innerPacket.Dst.String(),
			Type: model2.EndpointTypePod,
			Port: innerPacket.Dport,
		}, innerPacket.Protocol, pod)
	}

	upstream.DestinationAttribute = model2.SimpleLinkAttribute{Interface: iif}

	err := h.route.Assert(h.net, ack(upstream.Packet))
	if err != nil {
		return nil, err
	}

	h.net.AssertNetfilterServe(*upstream.Packet, iif)
	h.net.AssertListen(net.ParseIP(dst.IP), dst.Port, protocol)
	h.netNode.DoAction(model2.ActionServe(upstream))

	return nil, nil
}

func (h *flannelHost) detectIif(upstream *model2.Link) string {
	if upstream == nil {
		return ""
	}

	if upstream.Type == model2.LinkVeth {
		return h.bridge
	}

	return h.iface

}

func (h *flannelHost) basicCheck() error {
	node, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return err
	}
	h.k8s.AssertNode(node)

	h.net.AssertDefaultRule()
	h.net.AssertNoPolicyRoute()
	h.net.AssertDefaultAccept()
	h.net.AssertNetDevice(h.iface, netstack2.Interface{
		MTU:   1500,
		State: netstack2.LinkUP,
	})
	h.net.AssertHostBridge(h.bridge)
	h.net.AssertSysctls(map[string]string{
		"net.bridge.bridge-nf-call-iptables":                 "1",
		"net.ipv4.ip_forward":                                "1",
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", h.bridge): "1",
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", h.iface):  "1",
	}, model2.SuspicionLevelFatal)

	return nil
}

func (h *flannelHost) initRoute() error {
	interfaceDev, ok := lo.Find(h.nodeInfo.NetNS.Interfaces, func(i netstack2.Interface) bool { return i.Name == h.iface })
	if !ok {
		return fmt.Errorf("can not find interface named %s", h.iface)
	}

	ip, mask := netstack2.GetDefaultIPv4(&interfaceDev)
	cidr := net.IPNet{
		IP:   ip,
		Mask: mask,
	}

	routes := map[string]assertions2.RouteAssertion{
		h.podCIDR.String(): {Dev: &h.bridge, Scope: utils.ToPointer(netstack2.ScopeLink)},
		cidr.String():      {Dev: &h.iface, Scope: utils.ToPointer(netstack2.ScopeLink)},
	}

	if h.gateway != nil && !h.gateway.IsUnspecified() {
		routes["0.0.0.0/0"] = assertions2.RouteAssertion{
			Dev:   &h.iface,
			Scope: utils.ToPointer(netstack2.ScopeUniverse),
			Gw:    &h.gateway,
		}
	}

	node, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return err
	}

	h.route = newFlannelRoute(routes, h.podCIDR, h.clusterCIDR, h.net, node, h.ipCache)
	return nil
}

func (h *flannelHost) masquerade(pkt *model2.Packet) error {
	//flannel masquerade rule
	//
	//if src match cluster cidr and dst match cluster cidr：
	//do nothing
	//
	//if src match cluster cidr and dst not match 224.0.0.0/4(multicast):
	//masquerade
	//
	//if src not match cluster cidr and dst match pod cidr (on current node):
	//do nothing
	//
	//if src not match cluster cidr and dst match cluster cidr:
	//masquerade

	if !h.ipMasq {
		return nil
	}

	if h.clusterCIDR.Contains(pkt.Src) && h.clusterCIDR.Contains(pkt.Dst) {
		return nil
	}

	_, cidr, _ := net.ParseCIDR("224.0.0.0/4")
	if h.clusterCIDR.Contains(pkt.Src) && !cidr.Contains(pkt.Dst) {
		return h.doMasquerade(pkt)
	}

	if !h.clusterCIDR.Contains(pkt.Src) && h.podCIDR.Contains(pkt.Dst) {
		return nil
	}

	if !h.clusterCIDR.Contains(pkt.Src) && h.clusterCIDR.Contains(pkt.Dst) {
		return h.doMasquerade(pkt)
	}

	return nil
}

func (h *flannelHost) doMasquerade(pkt *model2.Packet) error {
	ip, _, err := h.nodeInfo.Router.RouteSrc(pkt, "", "")
	if err != nil {
		return err
	}
	pkt.Src = net.ParseIP(ip)
	return nil
}

func (h *flannelHost) checkRoute(pkt *model2.Packet) error {
	err := h.route.Assert(h.net, pkt)
	if err != nil {
		return err
	}
	err = h.route.Assert(h.net, ack(pkt))
	if err != nil {
		return err
	}
	return nil
}
