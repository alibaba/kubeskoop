package plugin

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
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

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CalicoNetworkMode string

const (
	CalicoNetworkModelBGP  CalicoNetworkMode = "BGP"
	CalicoNetworkModeIPIP  CalicoNetworkMode = "IPIP"
	CalicoNetworkModeVXLan CalicoNetworkMode = "VXLan"

	CalicoTunnelInterface = "tunl0"
)

type CalicoPluginOptions struct {
	InfraShim        network.InfraShim
	PodMTU           int
	IPIPPodMTU       int
	ServiceProcessor service.Processor
	Interface        string
}

type calicoPlugin struct {
	calicoClient     *clientset.Clientset
	serviceProcessor service.Processor
	podMTU           int
	ipipPodMTU       int

	infraShim network.InfraShim
	ipCache   *k8s2.IPCache

	hostOptions *calicoHostOptions
}

const (
	CalicoDefaultInterfacePrefix = "cali"
)

func calicoVethName(namespace, name string) string {
	h := sha1.New()
	h.Write([]byte(fmt.Sprintf("%s.%s", namespace, name)))
	return fmt.Sprintf("%s%s", CalicoDefaultInterfacePrefix, hex.EncodeToString(h.Sum(nil))[:11])
}

func getIPPool(ipPools []calicov3.IPPool, ip net.IP) *calicov3.IPPool {
	if ip == nil {
		return nil
	}

	matchedPool, ok := lo.Find(ipPools, func(pool calicov3.IPPool) bool {
		_, ipNet, _ := net.ParseCIDR(pool.Spec.CIDR)
		return ipNet.Contains(ip)
	})

	if !ok {
		return nil
	}

	return &matchedPool
}

func (c *calicoPlugin) CreatePod(pod *k8s2.Pod) (model2.NetNodeAction, error) {
	mtu := c.podMTU

	k8sPod, err := c.ipCache.GetPodFromName(pod.Namespace, pod.PodName)
	if err != nil {
		return nil, err
	}
	if k8sPod == nil {
		return nil, fmt.Errorf("cannot find pod %s/%s", pod.Namespace, pod.PodName)
	}
	pool := getIPPool(c.hostOptions.IPPools, net.ParseIP(k8sPod.Status.PodIP))
	if pool != nil {
		if pool.Spec.IPIPMode != calicov3.IPIPModeNever {
			mtu = c.ipipPodMTU
		}
	}

	return newSimpleVEthPod(pod, c.ipCache, mtu, "eth0")
}

func (c *calicoPlugin) CreateNode(node *k8s2.NodeInfo) (model2.NetNodeAction, error) {
	calicoHost, err := newCalicoHost(c.ipCache, node, c.infraShim, c.serviceProcessor, c.hostOptions)
	if err != nil {
		return nil, err
	}

	return &BasePluginNode{
		NetNode:          calicoHost.netNode,
		IPCache:          c.ipCache,
		SimplePluginNode: calicoHost,
	}, nil
}

type calicoRoute struct {
	*route
	iface    string
	ipCache  *k8s2.IPCache
	nodeName string
	ipPools  []calicov3.IPPool
	localNet *assertions2.NetstackAssertion
	network  *net.IPNet
}

func newCalicoRoute(parentRoute map[string]assertions2.RouteAssertion, ipCache *k8s2.IPCache, iface string, nodeName string,
	ipPools []calicov3.IPPool, localNetAssertion *assertions2.NetstackAssertion, network *net.IPNet) *calicoRoute {
	route := &calicoRoute{
		route:    newRoute(parentRoute),
		iface:    iface,
		ipCache:  ipCache,
		nodeName: nodeName,
		ipPools:  ipPools,
		localNet: localNetAssertion,
		network:  network,
	}

	return route
}

func (r *calicoRoute) getNetworkModeFromIPPool(pool *calicov3.IPPool, dstNodeIP net.IP) CalicoNetworkMode {
	if pool == nil {
		return CalicoNetworkModelBGP
	}

	switch pool.Spec.IPIPMode {
	case "Always":
		return CalicoNetworkModeIPIP
	case "CrossSubnet":
		if r.network.Contains(dstNodeIP) {
			return CalicoNetworkModelBGP
		}
		return CalicoNetworkModeIPIP
	case "Never":
		return CalicoNetworkModelBGP
	default:
		return CalicoNetworkModelBGP
	}
}

func (r *calicoRoute) AssertLocalPodRoute(pkt *model2.Packet, dstPod *v1.Pod) error {
	vethName := calicoVethName(dstPod.Namespace, dstPod.Name)
	assertion := assertions2.RouteAssertion{
		Dev:   &vethName,
		Type:  utils.ToPointer(netstack2.RtnUnicast),
		Scope: utils.ToPointer(netstack2.ScopeLink),
	}
	return r.localNet.AssertRoute(assertion, *pkt, "", "")
}

func (r *calicoRoute) AssertRemoteRoute(pkt *model2.Packet, networkMode CalicoNetworkMode, dstPod *v1.Pod) error {
	assertion := assertions2.RouteAssertion{
		Scope: utils.ToPointer(netstack2.ScopeUniverse),
	}
	if dstPod != nil {
		hostIP := net.ParseIP(dstPod.Status.HostIP)
		assertion.Gw = &hostIP
	}
	switch networkMode {
	case CalicoNetworkModelBGP:
		assertion.Dev = &r.iface
	case CalicoNetworkModeIPIP:
		assertion.Dev = utils.ToPointer(CalicoTunnelInterface)
		assertion.Protocol = utils.ToPointer(netstack2.RTProtBIRD)
	}

	return r.localNet.AssertRoute(assertion, *pkt, "", "")
}

func (r *calicoRoute) Assert(pkt *model2.Packet) error {
	ipPool := getIPPool(r.ipPools, pkt.Dst)
	if ipPool == nil {
		return r.route.Assert(r.localNet, pkt)
	}

	var hostIP net.IP

	pod, err := r.ipCache.GetPodFromIP(pkt.Dst.String())
	if err != nil {
		return err
	}

	if pod != nil {
		if pod.Spec.NodeName == r.nodeName {
			return r.AssertLocalPodRoute(pkt, pod)
		}
		hostIP = net.ParseIP(pod.Status.HostIP)
	}

	networkMode := r.getNetworkModeFromIPPool(ipPool, hostIP)
	return r.AssertRemoteRoute(pkt, networkMode, pod)
}

func NewCalicoPlugin(ctx *ctx.Context, options *CalicoPluginOptions) (Plugin, error) {
	client, err := clientset.NewForConfig(ctx.KubernetesRestClient())
	if err != nil {
		return nil, err
	}

	if options.ServiceProcessor == nil {
		return nil, fmt.Errorf("service processor must be provided")
	}

	ippools, err := client.ProjectcalicoV3().IPPools().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return &calicoPlugin{
		calicoClient:     client,
		infraShim:        options.InfraShim,
		podMTU:           options.PodMTU,
		ipipPodMTU:       options.IPIPPodMTU,
		ipCache:          ctx.ClusterConfig().IPCache,
		serviceProcessor: options.ServiceProcessor,
		hostOptions: &calicoHostOptions{
			Interface: options.Interface,
			IPPools:   ippools.Items,
		},
	}, nil
}

type calicoHostOptions struct {
	Interface string
	Gateway   net.IP
	IPPools   []calicov3.IPPool
}

type calicoHost struct {
	netNode  *model2.NetNode
	nodeInfo *k8s2.NodeInfo

	iface            string
	ipCache          *k8s2.IPCache
	infraShim        network.InfraShim
	serviceProcessor service.Processor
	network          *net.IPNet
	gateway          net.IP
	ipPools          []calicov3.IPPool

	net *assertions2.NetstackAssertion
	k8s *assertions2.KubernetesAssertion

	route *calicoRoute
}

func newCalicoHost(ipCache *k8s2.IPCache, nodeInfo *k8s2.NodeInfo, infraShim network.InfraShim,
	serviceProcessor service.Processor, options *calicoHostOptions) (*calicoHost, error) {

	netNode := model2.NewNetNode(nodeInfo.NodeName, model2.NetNodeTypeNode)
	assertion := assertions2.NewNetstackAssertion(netNode, &nodeInfo.NetNS)
	k8sAssertion := assertions2.NewKubernetesAssertion(netNode)

	iface, ok := lo.Find(nodeInfo.Interfaces, func(i netstack2.Interface) bool { return i.Name == options.Interface })
	if !ok {
		return nil, fmt.Errorf("cannot find interface %s", options.Interface)
	}
	ip, mask := netstack2.GetDefaultIPv4(&iface)
	ipNet := &net.IPNet{IP: ip, Mask: mask}

	host := &calicoHost{
		netNode:          netNode,
		nodeInfo:         nodeInfo,
		iface:            options.Interface,
		ipCache:          ipCache,
		serviceProcessor: serviceProcessor,
		network:          ipNet,
		gateway:          options.Gateway,
		ipPools:          options.IPPools,
		net:              assertion,
		k8s:              k8sAssertion,
	}

	err := host.initRoute()
	if err != nil {
		return nil, err
	}

	err = host.basicCheck()
	if err != nil {
		return nil, err
	}

	return host, nil
}

func (h *calicoHost) initRoute() error {
	routes := map[string]assertions2.RouteAssertion{
		h.network.String(): {Dev: &h.iface, Scope: utils.ToPointer(netstack2.ScopeLink)},
	}

	if h.gateway != nil && !h.gateway.IsUnspecified() {
		routes["0.0.0.0/0"] = assertions2.RouteAssertion{
			Dev:   &h.iface,
			Scope: utils.ToPointer(netstack2.ScopeUniverse),
			Gw:    &h.gateway,
		}
	}

	h.route = newCalicoRoute(routes, h.ipCache, h.iface, h.nodeInfo.NodeName, h.ipPools, h.net, h.network)
	return nil
}

func (h *calicoHost) basicCheck() error {
	h.net.AssertDefaultRule()
	h.net.AssertNoPolicyRoute()
	h.net.AssertNetDevice(h.iface, netstack2.Interface{MTU: 1500, State: netstack2.LinkUP})
	h.net.AssertSysctls(map[string]string{
		"net.ipv4.ip_forward":                               "1",
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", h.iface): "1",
	}, model2.SuspicionLevelFatal)
	return nil
}

func (h *calicoHost) transmissionToPod(pkt *model2.Packet, pod *v1.Pod, iif string) (model2.Transmission, error) {
	if !pod.Spec.HostNetwork && pod.Spec.NodeName == h.nodeInfo.NodeName {
		// send to local pod
		err := h.checkRoute(pkt)
		if err != nil {
			return model2.Transmission{}, err
		}

		ifName := calicoVethName(pod.Namespace, pod.Name)
		h.assertInterface(ifName)

		pktOut := pkt.DeepCopy()
		link := &model2.Link{
			Type:            model2.LinkVeth,
			Source:          h.netNode,
			Packet:          pktOut,
			SourceAttribute: model2.SimpleLinkAttribute{Interface: ifName},
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

	node, err := h.ipCache.GetNodeFromIP(pod.Status.HostIP)
	if err != nil {
		return model2.Transmission{}, err
	}

	return h.transmissionToNode(pkt, node, iif)
}

func (h *calicoHost) transmissionToNode(pkt *model2.Packet, node *v1.Node, iif string) (model2.Transmission, error) {
	err := h.checkRoute(pkt)
	if err != nil {
		return model2.Transmission{}, err
	}

	pktOut := pkt.DeepCopy()
	err = h.doMasquerade(pktOut)
	if err != nil {
		return model2.Transmission{}, nil
	}

	if h.infraShim != nil {
		dstNode, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
		if err != nil {
			return model2.Transmission{}, err
		}
		suspicions, err := h.infraShim.NodeToNode(dstNode, h.iface, node, pktOut)
		if err != nil {
			return model2.Transmission{}, err
		}
		h.netNode.Suspicions = append(h.netNode.Suspicions, suspicions...)
	}

	ipPool := getIPPool(h.ipPools, pkt.Dst)
	networkMode := h.route.getNetworkModeFromIPPool(ipPool, pkt.Dst)
	oif := h.iface
	if networkMode == CalicoNetworkModeIPIP {
		oif = CalicoTunnelInterface
		h.assertInterface(oif)
	}
	// encap packet
	pktOut, err = h.Encap(pktOut, node, networkMode)
	if err != nil {
		return model2.Transmission{}, err
	}

	link := &model2.Link{
		Type:            model2.LinkInfra,
		Source:          h.netNode,
		Packet:          pktOut,
		SourceAttribute: model2.SimpleLinkAttribute{Interface: oif},
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

func (h *calicoHost) transmissionToExternal(pkt *model2.Packet, iif string) (model2.Transmission, error) {
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

func (h *calicoHost) to(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, transmit transmissionFunc) ([]model2.Transmission, error) {
	iif, err := h.detectIif(upstream)
	if err != nil {
		return nil, err
	}

	var action *model2.Action
	var transmission model2.Transmission
	var pkt *model2.Packet
	var nfAssertionFunc func(pktIn model2.Packet, pktOut []model2.Packet, iif string)

	if upstream != nil {
		if upstream.Type == model2.LinkVeth {
			h.assertInterface(iif)
		}

		upstream.Destination = h.netNode
		upstream.DestinationAttribute = model2.SimpleLinkAttribute{Interface: iif}
		pkt = h.Decap(upstream.Packet)
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
			if err == netstack2.ErrNoRouteToHost {
				h.netNode.AddSuspicion(model2.SuspicionLevelFatal, fmt.Sprintf("no route to host: %v", dst))
				return nil, &assertions2.CannotBuildTransmissionError{
					SrcNode: h.netNode,
					Err:     fmt.Errorf("no route to host: %v", err),
				}
			}
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

func (h *calicoHost) ToPod(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, pod *v1.Pod) ([]model2.Transmission, error) {
	makeLink := func(pkt *model2.Packet, iif string) (model2.Transmission, error) {
		return h.transmissionToPod(pkt, pod, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *calicoHost) ToHost(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, node *v1.Node) ([]model2.Transmission, error) {
	makeLink := func(pkt *model2.Packet, iif string) (model2.Transmission, error) {
		return h.transmissionToNode(pkt, node, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *calicoHost) ToService(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol, service *v1.Service) ([]model2.Transmission, error) {
	iif, err := h.detectIif(upstream)
	if err != nil {
		return nil, err
	}
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

func (h *calicoHost) ToExternal(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol) ([]model2.Transmission, error) {
	makeLink := func(pkt *model2.Packet, iif string) (model2.Transmission, error) {
		return h.transmissionToExternal(pkt, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *calicoHost) Serve(upstream *model2.Link, dst model2.Endpoint, protocol model2.Protocol) ([]model2.Transmission, error) {
	iif, err := h.detectIif(upstream)
	if err != nil {
		return nil, err
	}

	if upstream.Packet.Encap != nil {
		h.net.AssertNetfilterServe(*upstream.Packet, iif)
		innerPacket := h.Decap(upstream.Packet)
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

	err = h.route.Assert(ack(upstream.Packet))
	if err != nil {
		return nil, err
	}
	upstream.DestinationAttribute = model2.SimpleLinkAttribute{Interface: iif}

	h.net.AssertNetfilterServe(*upstream.Packet, iif)
	h.net.AssertListen(net.ParseIP(dst.IP), dst.Port, protocol)
	h.netNode.DoAction(model2.ActionServe(upstream))

	return nil, nil
}

func (h *calicoHost) checkRoute(pkt *model2.Packet) error {
	err := h.route.Assert(pkt)
	if err != nil {
		return err
	}
	err = h.route.Assert(ack(pkt))
	if err != nil {
		return err
	}
	return nil
}

func (h *calicoHost) assertInterface(ifName string) {
	if ifName == CalicoTunnelInterface {
		h.net.AssertDefaultIPIPTunnel(ifName)
	}
	h.net.AssertNetDevice(ifName, netstack2.Interface{State: netstack2.LinkUP})
	h.net.AssertSysctls(map[string]string{
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", ifName): "1",
	}, model2.SuspicionLevelWarning)
}

func (h *calicoHost) Encap(pkt *model2.Packet, nextNode *v1.Node, mode CalicoNetworkMode) (*model2.Packet, error) {
	if mode == CalicoNetworkModeIPIP {
		nextNodeIP := nextNode.Annotations["projectcalico.org/IPv4Address"]
		if nextNodeIP == "" {
			return nil, fmt.Errorf("node %q does not have projectcalico.org/IPv4Address annotation", nextNode.Name)
		}
		ip, _, err := net.ParseCIDR(nextNodeIP)
		if err != nil {
			return nil, err
		}

		pktOut := &model2.Packet{
			Src:      h.network.IP,
			Dst:      ip,
			Dport:    pkt.Dport,
			Protocol: model2.IPv4,
			Encap:    pkt,
		}
		return pktOut, nil
	}

	return pkt, nil
}

func (h *calicoHost) Decap(pkt *model2.Packet) *model2.Packet {
	if pkt.Encap != nil {
		return pkt.Encap
	}
	return pkt
}

func (h *calicoHost) masquerade(pkt *model2.Packet) error {
	ippoolSrc := getIPPool(h.ipPools, pkt.Src)
	if ippoolSrc == nil {
		return nil
	}

	if !ippoolSrc.Spec.NATOutgoing {
		return nil
	}

	ipPoolDst := getIPPool(h.ipPools, pkt.Dst)
	if ipPoolDst != nil {
		return nil
	}

	node, err := h.ipCache.GetNodeFromIP(pkt.Dst.String())
	if err != nil {
		return err
	}

	if node != nil {
		return nil
	}

	return h.doMasquerade(pkt)
}

func (h *calicoHost) doMasquerade(pkt *model2.Packet) error {
	srcPool := getIPPool(h.ipPools, pkt.Src)
	if srcPool == nil {
		return nil
	}

	if !srcPool.Spec.NATOutgoing {
		return nil
	}

	dstPool := getIPPool(h.ipPools, pkt.Dst)
	if dstPool != nil {
		return nil
	}

	ip, _, err := h.nodeInfo.Router.RouteSrc(pkt, "", "")
	if err != nil {
		return err
	}
	pkt.Src = net.ParseIP(ip)
	return nil
}

func (h *calicoHost) detectIif(upstream *model2.Link) (string, error) {
	if upstream == nil {
		return "", nil
	}

	if upstream.Type == model2.LinkVeth {
		// fixme: should not depend on the format of pod id
		names := strings.Split(upstream.Source.GetID(), "/")
		pod, err := h.ipCache.GetPodFromName(names[0], names[1])
		if err != nil {
			return "", err
		}

		return calicoVethName(pod.Namespace, pod.Name), nil
	}

	if upstream.Packet.Encap != nil && upstream.Packet.Protocol == model2.IPv4 {
		return CalicoTunnelInterface, nil
	}

	return h.iface, nil
}
