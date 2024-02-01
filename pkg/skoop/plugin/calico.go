package plugin

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"k8s.io/klog/v2"

	"github.com/spf13/pflag"

	"github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CalicoConfig struct {
	HostMTU    int
	PodMTU     int
	IPIPPodMTU int
	Interface  string
}

func (c *CalicoConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&c.Interface, "calico-host-interface", "", "",
		"Host interface for calico plugin.")
	fs.IntVarP(&c.HostMTU, "calico-host-mtu", "", 1500,
		"Host MTU for calico plugin. Host interface MTU in BGP mode.")
	fs.IntVarP(&c.PodMTU, "calico-pod-mtu", "", 1500,
		"Pod MTU for calico plugin. Pod interface MTU in BGP mode.")
	fs.IntVarP(&c.IPIPPodMTU, "calico-ipip-pod-mtu", "", 1480,
		"Pod MTU for calico plugin. Pod interface MTU in IPIP mode.")
}

func (c *CalicoConfig) Validate() error {
	return nil
}

var Calico = &CalicoConfig{}

func init() {
	ctx.RegisterConfigBinder("Calico plugin", Calico)
}

type CalicoNetworkMode string

const (
	CalicoNetworkModelBGP  CalicoNetworkMode = "BGP"
	CalicoNetworkModeIPIP  CalicoNetworkMode = "IPIP"
	CalicoNetworkModeVXLan CalicoNetworkMode = "VXLan"

	CalicoTunnelInterface = "tunl0"
)

type CalicoPluginOptions struct {
	InfraShim        network.InfraShim
	HostMTU          int
	PodMTU           int
	IPIPPodMTU       int
	ServiceProcessor service.Processor
	Interface        string
}

type calicoPlugin struct {
	serviceProcessor service.Processor
	podMTU           int
	ipipPodMTU       int

	infraShim network.InfraShim
	ipCache   *k8s.IPCache

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

func listIPPools(ctx *ctx.Context) ([]calicov3.IPPool, error) {
	client, err := clientset.NewForConfig(ctx.KubernetesRestClient())
	if err != nil {
		return nil, err
	}

	ippools, err := client.ProjectcalicoV3().IPPools().List(context.TODO(), metav1.ListOptions{})
	if err == nil {
		return ippools.Items, nil
	}

	klog.V(5).Infof("not able to list projectcalico.org/v3 ippools, error %s, fallback to list crd.", err)
	dynClient, err := dynamic.NewForConfig(ctx.KubernetesRestClient())
	if err != nil {
		return nil, err
	}

	gvr := schema.GroupVersionResource{
		Group:    "crd.projectcalico.org",
		Version:  "v1",
		Resource: "ippools",
	}
	list, err := dynClient.Resource(gvr).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	str, err := list.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var ippoolList calicov3.IPPoolList
	err = json.Unmarshal(str, &ippoolList)
	if err != nil {
		return nil, err
	}

	return ippools.Items, err
}

func (c *calicoPlugin) CreatePod(pod *k8s.Pod) (model.NetNodeAction, error) {
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

func (c *calicoPlugin) CreateNode(node *k8s.NodeInfo) (model.NetNodeAction, error) {
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
	ipCache  *k8s.IPCache
	nodeName string
	ipPools  []calicov3.IPPool
	localNet *assertions.NetstackAssertion
	network  *net.IPNet
}

func newCalicoRoute(parentRoute map[string]assertions.RouteAssertion, ipCache *k8s.IPCache, iface string, nodeName string,
	ipPools []calicov3.IPPool, localNetAssertion *assertions.NetstackAssertion, network *net.IPNet) *calicoRoute {
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

func (r *calicoRoute) AssertLocalPodRoute(pkt *model.Packet, dstPod *v1.Pod) error {
	vethName := calicoVethName(dstPod.Namespace, dstPod.Name)
	assertion := assertions.RouteAssertion{
		Dev:   &vethName,
		Type:  utils.ToPointer(netstack.RtnUnicast),
		Scope: utils.ToPointer(netstack.ScopeLink),
	}
	return r.localNet.AssertRoute(assertion, *pkt, "", "")
}

func (r *calicoRoute) AssertRemoteRoute(pkt *model.Packet, networkMode CalicoNetworkMode, dstPod *v1.Pod) error {
	assertion := assertions.RouteAssertion{
		Scope: utils.ToPointer(netstack.ScopeUniverse),
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
		assertion.Protocol = utils.ToPointer(netstack.RTProtBIRD)
	}

	return r.localNet.AssertRoute(assertion, *pkt, "", "")
}

func (r *calicoRoute) Assert(pkt *model.Packet) error {
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

func NewCalicoPluginWithOptions(ctx *ctx.Context, options *CalicoPluginOptions) (Plugin, error) {
	if options.ServiceProcessor == nil {
		return nil, fmt.Errorf("service processor must be provided")
	}

	ippools, err := listIPPools(ctx)
	if err != nil {
		return nil, err
	}

	return &calicoPlugin{
		infraShim:        options.InfraShim,
		podMTU:           options.PodMTU,
		ipipPodMTU:       options.IPIPPodMTU,
		ipCache:          ctx.ClusterConfig().IPCache,
		serviceProcessor: options.ServiceProcessor,
		hostOptions: &calicoHostOptions{
			Interface: options.Interface,
			IPPools:   ippools,
			MTU:       options.HostMTU,
		},
	}, nil
}

func NewCalicoPlugin(ctx *ctx.Context, serviceProcessor service.Processor, infraShim network.InfraShim) (Plugin, error) {
	options := &CalicoPluginOptions{
		InfraShim:        infraShim,
		PodMTU:           Calico.PodMTU,
		HostMTU:          Calico.HostMTU,
		IPIPPodMTU:       Calico.IPIPPodMTU,
		ServiceProcessor: serviceProcessor,
		Interface:        Calico.Interface,
	}

	return NewCalicoPluginWithOptions(ctx, options)
}

type calicoHostOptions struct {
	Interface string
	MTU       int
	Gateway   net.IP
	IPPools   []calicov3.IPPool
}

type calicoHost struct {
	netNode  *model.NetNode
	nodeInfo *k8s.NodeInfo

	iface            string
	mtu              int
	ipCache          *k8s.IPCache
	infraShim        network.InfraShim
	serviceProcessor service.Processor
	network          *net.IPNet
	gateway          net.IP
	ipPools          []calicov3.IPPool

	net *assertions.NetstackAssertion
	k8s *assertions.KubernetesAssertion

	route *calicoRoute
}

func newCalicoHost(ipCache *k8s.IPCache, nodeInfo *k8s.NodeInfo, infraShim network.InfraShim,
	serviceProcessor service.Processor, options *calicoHostOptions) (*calicoHost, error) {

	netNode := model.NewNetNode(nodeInfo.NodeName, model.NetNodeTypeNode)
	assertion := assertions.NewNetstackAssertion(netNode, &nodeInfo.NetNS)
	k8sAssertion := assertions.NewKubernetesAssertion(netNode)

	host := &calicoHost{
		netNode:          netNode,
		nodeInfo:         nodeInfo,
		iface:            options.Interface,
		mtu:              options.MTU,
		ipCache:          ipCache,
		serviceProcessor: serviceProcessor,
		gateway:          options.Gateway,
		ipPools:          options.IPPools,
		net:              assertion,
		k8s:              k8sAssertion,
		infraShim:        infraShim,
	}

	if host.iface == "" {
		host.iface = netstack.LookupDefaultIfaceName(nodeInfo.NetNSInfo.Interfaces)
		if host.iface == "" {
			return nil, fmt.Errorf("cannot lookup default host interface, please manually specify it via --calico-host-interface")
		}
		klog.V(5).Infof("detected host interface %s on node %s", host.iface, nodeInfo.NodeName)
	}

	iface, ok := lo.Find(nodeInfo.Interfaces, func(i netstack.Interface) bool { return i.Name == host.iface })
	if !ok {
		return nil, fmt.Errorf("cannot find interface %s", options.Interface)
	}

	ip, mask := netstack.GetDefaultIPv4(&iface)
	ipNet := &net.IPNet{IP: ip, Mask: mask}
	host.network = ipNet

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
	routes := map[string]assertions.RouteAssertion{
		h.network.String(): {Dev: &h.iface, Scope: utils.ToPointer(netstack.ScopeLink)},
	}

	if h.gateway != nil && !h.gateway.IsUnspecified() {
		routes["0.0.0.0/0"] = assertions.RouteAssertion{
			Dev:   &h.iface,
			Scope: utils.ToPointer(netstack.ScopeUniverse),
			Gw:    &h.gateway,
		}
	}

	h.route = newCalicoRoute(routes, h.ipCache, h.iface, h.nodeInfo.NodeName, h.ipPools, h.net, h.network)
	return nil
}

func (h *calicoHost) basicCheck() error {
	h.net.AssertDefaultRule()
	h.net.AssertNoPolicyRoute()
	h.net.AssertNetDevice(h.iface, netstack.Interface{MTU: h.mtu, State: netstack.LinkUP})
	h.net.AssertSysctls(map[string]string{
		"net.ipv4.ip_forward": "1",
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", utils.ConvertNICNameInSysctls(h.iface)): "1",
	}, model.SuspicionLevelFatal)
	return nil
}

func (h *calicoHost) transmissionToPod(pkt *model.Packet, pod *v1.Pod, iif string) (model.Transmission, error) {
	if !pod.Spec.HostNetwork && pod.Spec.NodeName == h.nodeInfo.NodeName {
		// send to local pod
		err := h.checkRoute(pkt)
		if err != nil {
			return model.Transmission{}, err
		}

		ifName := calicoVethName(pod.Namespace, pod.Name)
		h.assertInterface(ifName)

		pktOut := pkt.DeepCopy()
		link := &model.Link{
			Type:            model.LinkVeth,
			Source:          h.netNode,
			Packet:          pktOut,
			SourceAttribute: model.SimpleLinkAttribute{Interface: ifName},
		}
		nextHop := model.Hop{
			Type: model.NetNodeTypePod,
			ID:   pkt.Dst.String(),
		}
		return model.Transmission{
			NextHop: nextHop,
			Link:    link,
		}, nil
	}

	node, err := h.ipCache.GetNodeFromIP(pod.Status.HostIP)
	if err != nil {
		return model.Transmission{}, err
	}

	return h.transmissionToNode(pkt, node, iif)
}

func (h *calicoHost) transmissionToNode(pkt *model.Packet, node *v1.Node, _ string) (model.Transmission, error) {
	err := h.checkRoute(pkt)
	if err != nil {
		return model.Transmission{}, err
	}

	pktOut := pkt.DeepCopy()
	err = h.doMasquerade(pktOut)
	if err != nil {
		return model.Transmission{}, nil
	}

	if h.infraShim != nil {
		dstNode, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
		if err != nil {
			return model.Transmission{}, err
		}
		suspicions, err := h.infraShim.NodeToNode(dstNode, h.iface, node, pktOut)
		if err != nil {
			return model.Transmission{}, err
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
		return model.Transmission{}, err
	}

	link := &model.Link{
		Type:            model.LinkInfra,
		Source:          h.netNode,
		Packet:          pktOut,
		SourceAttribute: model.SimpleLinkAttribute{Interface: oif},
	}

	nextHop := model.Hop{
		Type: model.NetNodeTypeNode,
		ID:   node.Name,
	}

	return model.Transmission{
		NextHop: nextHop,
		Link:    link,
	}, nil
}

func (h *calicoHost) transmissionToExternal(pkt *model.Packet, _ string) (model.Transmission, error) {
	err := h.checkRoute(pkt)
	if err != nil {
		return model.Transmission{}, err
	}
	pktOut := pkt.DeepCopy()
	err = h.masquerade(pktOut)
	if err != nil {
		return model.Transmission{}, err
	}

	link := &model.Link{
		Type:            model.LinkInfra,
		Source:          h.netNode,
		Packet:          pktOut,
		SourceAttribute: model.SimpleLinkAttribute{Interface: h.iface},
	}
	nextHop := model.Hop{
		Type: model.NetNodeTypeExternal,
		ID:   pktOut.Dst.String(),
	}

	return model.Transmission{
		NextHop: nextHop,
		Link:    link,
	}, nil

}

func (h *calicoHost) to(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, transmit transmissionFunc) ([]model.Transmission, error) {
	iif, err := h.detectIif(upstream)
	if err != nil {
		return nil, err
	}

	var action *model.Action
	var transmission model.Transmission
	var pkt *model.Packet
	var nfAssertionFunc func(pktIn model.Packet, pktOut []model.Packet, iif string)

	if upstream != nil {
		if upstream.Type == model.LinkVeth {
			h.assertInterface(iif)
		}

		upstream.Destination = h.netNode
		upstream.DestinationAttribute = model.SimpleLinkAttribute{Interface: iif}
		pkt = h.Decap(upstream.Packet)
		transmission, err = transmit(pkt, iif)
		if err != nil {
			return nil, err
		}
		if transmission.Link == nil {
			return nil, nil
		}
		nfAssertionFunc = h.net.AssertNetfilterForward
		action = model.ActionForward(upstream, []*model.Link{transmission.Link})
	} else {
		pkt = &model.Packet{
			Dst:      net.ParseIP(dst.IP),
			Dport:    dst.Port,
			Protocol: protocol,
		}
		addr, _, err := h.nodeInfo.Router.RouteSrc(pkt, "", "")
		if err != nil {
			if err == netstack.ErrNoRouteToHost {
				h.netNode.AddSuspicion(model.SuspicionLevelFatal, fmt.Sprintf("no route to host: %v", dst))
				return nil, &assertions.CannotBuildTransmissionError{
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
		action = model.ActionSend([]*model.Link{transmission.Link})
	}

	pktOut := transmission.Link.Packet
	nfAssertionFunc(*pkt, []model.Packet{*pktOut}, iif)

	h.netNode.DoAction(action)
	return []model.Transmission{transmission}, nil
}

func (h *calicoHost) ToPod(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, pod *v1.Pod) ([]model.Transmission, error) {
	makeLink := func(pkt *model.Packet, iif string) (model.Transmission, error) {
		return h.transmissionToPod(pkt, pod, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *calicoHost) ToHost(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, node *v1.Node) ([]model.Transmission, error) {
	makeLink := func(pkt *model.Packet, iif string) (model.Transmission, error) {
		return h.transmissionToNode(pkt, node, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *calicoHost) ToService(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, service *v1.Service) ([]model.Transmission, error) {
	iif, err := h.detectIif(upstream)
	if err != nil {
		return nil, err
	}
	var pkt *model.Packet
	if upstream != nil {
		upstream.DestinationAttribute = model.SimpleLinkAttribute{Interface: iif}
		pkt = upstream.Packet
	} else {
		pkt = &model.Packet{
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

	node, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return nil, err
	}

	backends := h.serviceProcessor.Process(*pkt, service, node)
	if len(backends) == 0 {
		h.netNode.Suspicions = append(h.netNode.Suspicions, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("service %s/%s has no valid endpoint", service.Namespace, service.Name),
		})
		h.netNode.DoAction(model.ActionService(upstream, []*model.Link{}))
		return nil, &assertions.CannotBuildTransmissionError{
			SrcNode: h.netNode,
			Err:     fmt.Errorf("service %s/%s has no valid endpoint", service.Namespace, service.Name),
		}
	}

	if err := h.serviceProcessor.Validate(*pkt, backends, h.nodeInfo.NetNS); err != nil {
		h.netNode.Suspicions = append(h.netNode.Suspicions, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("validate endpoint of service %s/%s failed: %s", service.Namespace, service.Name, err),
		})
	}

	var nfAssertion func(pktIn model.Packet, pktOut []model.Packet, iif string)
	if upstream != nil {
		nfAssertion = h.net.AssertNetfilterForward
	} else {
		nfAssertion = h.net.AssertNetfilterSend
	}

	var transmissions []model.Transmission
	for _, backend := range backends {
		pktOut := &model.Packet{
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
		case model.EndpointTypePod:
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
		case model.EndpointTypeNode:
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

	links := lo.Map(transmissions, func(t model.Transmission, _ int) *model.Link { return t.Link })
	h.netNode.DoAction(model.ActionService(upstream, links))

	pktOutList := lo.Map(links, func(l *model.Link, _ int) model.Packet { return *l.Packet })
	nfAssertion(*pkt, pktOutList, iif)
	return transmissions, nil
}

func (h *calicoHost) ToExternal(upstream *model.Link, dst model.Endpoint, protocol model.Protocol) ([]model.Transmission, error) {
	makeLink := func(pkt *model.Packet, iif string) (model.Transmission, error) {
		return h.transmissionToExternal(pkt, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *calicoHost) Serve(upstream *model.Link, dst model.Endpoint, protocol model.Protocol) ([]model.Transmission, error) {
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
		return h.ToPod(upstream, model.Endpoint{
			IP:   innerPacket.Dst.String(),
			Type: model.EndpointTypePod,
			Port: innerPacket.Dport,
		}, innerPacket.Protocol, pod)
	}

	err = h.route.Assert(ack(upstream.Packet))
	if err != nil {
		return nil, err
	}
	upstream.DestinationAttribute = model.SimpleLinkAttribute{Interface: iif}

	h.net.AssertNetfilterServe(*upstream.Packet, iif)
	h.net.AssertListen(net.ParseIP(dst.IP), dst.Port, protocol)
	h.netNode.DoAction(model.ActionServe(upstream))

	return nil, nil
}

func (h *calicoHost) checkRoute(pkt *model.Packet) error {
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
	h.net.AssertNetDevice(ifName, netstack.Interface{State: netstack.LinkUP})
	h.net.AssertSysctls(map[string]string{
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", utils.ConvertNICNameInSysctls(ifName)): "1",
	}, model.SuspicionLevelWarning)
}

func (h *calicoHost) Encap(pkt *model.Packet, nextNode *v1.Node, mode CalicoNetworkMode) (*model.Packet, error) {
	if mode == CalicoNetworkModeIPIP {
		nextNodeIP := nextNode.Annotations["projectcalico.org/IPv4Address"]
		if nextNodeIP == "" {
			return nil, fmt.Errorf("node %q does not have projectcalico.org/IPv4Address annotation", nextNode.Name)
		}
		ip, _, err := net.ParseCIDR(nextNodeIP)
		if err != nil {
			return nil, err
		}

		pktOut := &model.Packet{
			Src:      h.network.IP,
			Dst:      ip,
			Dport:    pkt.Dport,
			Protocol: model.IPv4,
			Encap:    pkt,
		}
		return pktOut, nil
	}

	return pkt, nil
}

func (h *calicoHost) Decap(pkt *model.Packet) *model.Packet {
	if pkt.Encap != nil {
		return pkt.Encap
	}
	return pkt
}

func (h *calicoHost) masquerade(pkt *model.Packet) error {
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

func (h *calicoHost) doMasquerade(pkt *model.Packet) error {
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

func (h *calicoHost) detectIif(upstream *model.Link) (string, error) {
	if upstream == nil {
		return "", nil
	}

	if upstream.Type == model.LinkVeth {
		// fixme: should not depend on the format of pod id
		names := strings.Split(upstream.Source.GetID(), "/")
		pod, err := h.ipCache.GetPodFromName(names[0], names[1])
		if err != nil {
			return "", err
		}

		return calicoVethName(pod.Namespace, pod.Name), nil
	}

	if upstream.Packet.Encap != nil && upstream.Packet.Protocol == model.IPv4 {
		return CalicoTunnelInterface, nil
	}

	return h.iface, nil
}
