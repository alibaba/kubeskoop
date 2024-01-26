package plugin

import (
	"context"
	"fmt"
	"net"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"

	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
)

type FlannelConfig struct {
	BackendType string
	Bridge      string
	PodMTU      int
	IPMasq      bool
	Interface   string
}

var (
	supportedFlannelBackendType = []string{"host-gw", "vxlan", "alloc"}
)

func (f *FlannelConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&f.BackendType, "flannel-backend-type", "", "",
		"Backend type for flannel plugin, support host-gw,vxlan,alloc. If not set, it will auto detect from flannel config.")
	fs.StringVarP(&f.Bridge, "flannel-bridge", "", "cni0",
		"Bridge name for flannel plugin.")
	fs.IntVarP(&f.PodMTU, "flannel-pod-mtu", "", 0,
		"Pod MTU for flannel plugin. If not set, it will auto detect from flannel cni mode (1450 for vxlan, 1500 for others).")
	fs.BoolVarP(&f.IPMasq, "flannel-ip-masq", "", true,
		"Should do IP masquerade for flannel plugin.")
	fs.StringVarP(&f.Interface, "flannel-host-interface", "", "",
		"Host interface for flannel plugin.")
}

func (f *FlannelConfig) Validate() error {
	if f.BackendType != "" && !slices.Contains(supportedFlannelBackendType, f.BackendType) {
		return fmt.Errorf("unsupported flannel backed type %q, should be %s",
			f.BackendType, strings.Join(supportedFlannelBackendType, ","))
	}
	return nil
}

var Flannel = &FlannelConfig{}

func init() {
	ctx.RegisterConfigBinder("Flannel plugin", Flannel)
}

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

func getFlannelConfigMap(ctx *ctx.Context) (*v1.ConfigMap, error) {
	configMaps := []struct {
		Namespace string
		Name      string
	}{
		{"kube-flannel", "kube-flannel-cfg"},
		{"kube-system", "kube-flannel-cfg"},
	}

	var cm *v1.ConfigMap
	var err error
	for _, c := range configMaps {
		cm, err = ctx.KubernetesClient().CoreV1().
			ConfigMaps(c.Namespace).Get(context.TODO(), c.Name, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			break
		}
	}

	return cm, nil
}

func getFlannelCNIMode(ctx *ctx.Context) (FlannelBackendType, error) {
	cm, err := getFlannelConfigMap(ctx)
	if err != nil {
		return "", err
	}

	if cm == nil {
		klog.V(3).Infof("Can not detect flannel backend type, use \"host-gw\" as default.")
		return FlannelBackendTypeHostGW, nil
	}

	conf := cm.Data["net-conf.json"]
	if strings.Contains(conf, "vxlan") {
		return FlannelBackendTypeVxlan, nil
	}
	if strings.Contains(conf, "host-gw") {
		return FlannelBackendTypeHostGW, nil
	}
	if strings.Contains(conf, "alloc") {
		return FlannelBackendTypeAlloc, nil
	}

	klog.V(3).Infof("Can not detect flannel backend type or unsupported type, use \"host-gw\" as default.")
	return FlannelBackendTypeHostGW, nil
}

type flannelPlugin struct {
	hostOptions      *flannelHostOptions
	serviceProcessor service.Processor
	podMTU           int

	infraShim network.InfraShim
	ipCache   *k8s.IPCache
}

func (f *flannelPlugin) CreatePod(pod *k8s.Pod) (model.NetNodeAction, error) {
	return newSimpleVEthPod(pod, f.ipCache, f.podMTU, "eth0")
}

func (f *flannelPlugin) CreateNode(node *k8s.NodeInfo) (model.NetNodeAction, error) {
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

func NewFlannelPluginWithOptions(ctx *ctx.Context, options *FlannelPluginOptions) (Plugin, error) {
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

func NewFlannelPlugin(ctx *ctx.Context, serviceProcessor service.Processor, infraShim network.InfraShim) (Plugin, error) {
	options := &FlannelPluginOptions{
		InfraShim:        infraShim,
		Bridge:           Flannel.Bridge,
		Interface:        Flannel.Interface,
		PodMTU:           Flannel.PodMTU,
		IPMasq:           Flannel.IPMasq,
		ClusterCIDR:      ctx.ClusterConfig().ClusterCIDR,
		CNIMode:          FlannelBackendType(Flannel.BackendType),
		ServiceProcessor: serviceProcessor,
	}

	if options.CNIMode == "" {
		mode, err := getFlannelCNIMode(ctx)
		if err != nil {
			return nil, err
		}
		options.CNIMode = mode
	}

	if options.PodMTU == 0 {
		mtu := 1500
		if options.CNIMode == FlannelBackendTypeVxlan {
			mtu = 1450
		}
		options.PodMTU = mtu
	}

	return NewFlannelPluginWithOptions(ctx, options)
}

type flannelNodeInfo struct {
	Vtep        net.IP
	CIDR        *net.IPNet
	BackendType FlannelBackendType
	NodeIP      net.IP
	Dev         *netstack.Interface
	Route       assertions.RouteAssertion
}

type flannelRoute struct {
	*route
	localNetAssertion *assertions.NetstackAssertion
	localPodCIDR      *net.IPNet
	clusterCIDR       *net.IPNet
	localVTEP         net.IP
	ipCache           *k8s.IPCache
	nodeInfoCache     map[string]*flannelNodeInfo
	cniMode           FlannelBackendType
	iface             string
}

func newFlannelRoute(parentRoute map[string]assertions.RouteAssertion, localPodCIDR *net.IPNet,
	clusterCIDR *net.IPNet, localNetAssertion *assertions.NetstackAssertion, localNode *v1.Node, ipCache *k8s.IPCache,
	cniMode FlannelBackendType, iface string) *flannelRoute {
	route := &flannelRoute{
		route:             newRoute(parentRoute),
		localNetAssertion: localNetAssertion,
		localPodCIDR:      localPodCIDR,
		clusterCIDR:       clusterCIDR,
		localVTEP:         net.ParseIP(localNode.Annotations["flannel.alpha.coreos.com/public-ip"]),
		ipCache:           ipCache,
		nodeInfoCache:     map[string]*flannelNodeInfo{},
		cniMode:           cniMode,
		iface:             iface,
	}

	return route
}

func (r *flannelRoute) AssertBackend(pkt *model.Packet) error {
	hostInfo, err := r.getDstInfo(pkt)
	if err != nil {
		return err
	}

	if hostInfo != nil && hostInfo.Dev != nil {
		r.localNetAssertion.AssertNetDevice(hostInfo.Dev.Name, netstack.Interface{
			State: netstack.LinkUP,
			MTU:   hostInfo.Dev.MTU,
		})
		r.localNetAssertion.AssertSysctls(map[string]string{
			fmt.Sprintf("net.ipv4.conf.%s.forwarding",
				utils.ConvertNICNameInSysctls(hostInfo.Dev.Name)): "1",
		}, model.SuspicionLevelFatal)
		if hostInfo.BackendType == "vxlan" {
			err = r.localNetAssertion.AssertVxlanVtep(hostInfo.Vtep, hostInfo.NodeIP, flannelVxlanInterface)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *flannelRoute) Encap(opkt *model.Packet) (*model.Packet, error) {
	pkt := opkt
	hostInfo, err := r.getDstInfo(opkt)
	if err != nil {
		return nil, err
	}
	if hostInfo == nil {
		return pkt, nil
	}

	if hostInfo.BackendType == "vxlan" {
		pkt = &model.Packet{
			Src:      r.localVTEP,
			Sport:    pkt.Sport,
			Dst:      hostInfo.NodeIP,
			Dport:    8472,
			Protocol: model.UDP,
			Encap:    opkt.DeepCopy(),
		}
	}

	return pkt, nil
}

func (r *flannelRoute) Decap(opkt *model.Packet) (*model.Packet, error) {
	if opkt.Encap != nil {
		if !opkt.Dst.Equal(r.localVTEP) {
			return nil, fmt.Errorf("encap dst %s not match local vtep %s", opkt.Dst, r.localVTEP)
		}
		return opkt.Encap, nil
	}
	return opkt, nil
}

func (r *flannelRoute) getDstInfo(pkt *model.Packet) (*flannelNodeInfo, error) {
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
	var dev *netstack.Interface
	var vtep net.IP
	if strings.EqualFold(backendTypeAnnotation, string(FlannelBackendTypeVxlan)) {
		vtep = ip
		dev = &netstack.Interface{
			Name:   flannelVxlanInterface,
			MTU:    1450,
			Driver: "vxlan",
		}
		backendType = FlannelBackendTypeVxlan
	} else if strings.EqualFold(backendTypeAnnotation, string(FlannelBackendTypeHostGW)) {
		vtep = net.ParseIP(nextNodeIP)
		backendType = FlannelBackendTypeHostGW
	} else {
		backendType = r.cniMode
	}

	if dev == nil {
		dev = &netstack.Interface{
			Name: r.iface,
		}
	}

	route := assertions.RouteAssertion{}
	if vtep != nil && !vtep.IsUnspecified() {
		route.Gw = &vtep
	}

	if dev != nil {
		route.Dev = &dev.Name
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

func (r *flannelRoute) Assert(netAssertion *assertions.NetstackAssertion, pkt *model.Packet) error {
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
	nodeInfo         *k8s.NodeInfo
	netNode          *model.NetNode
	ipCache          *k8s.IPCache
	infraShim        network.InfraShim
	serviceProcessor service.Processor

	bridge      string
	clusterCIDR *net.IPNet
	podCIDR     *net.IPNet
	iface       string
	cniMode     FlannelBackendType
	ipMasq      bool
	gateway     net.IP

	net   *assertions.NetstackAssertion
	k8s   *assertions.KubernetesAssertion
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

func newFlannelHost(ipCache *k8s.IPCache, nodeInfo *k8s.NodeInfo, infraShim network.InfraShim,
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

	netNode := model.NewNetNode(nodeInfo.NodeName, model.NetNodeTypeNode)

	assertion := assertions.NewNetstackAssertion(netNode, &nodeInfo.NetNS)
	k8sAssertion := assertions.NewKubernetesAssertion(netNode)

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

	if host.iface == "" {
		host.iface = netstack.LookupDefaultIfaceName(nodeInfo.NetNSInfo.Interfaces)
		if host.iface == "" {
			return nil, fmt.Errorf("cannot lookup default host interface, please manually specify it via --flannel-host-interface")
		}
		klog.V(5).Infof("detected host interface %s on node %s", host.iface, host.nodeInfo.NodeName)
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

func (h *flannelHost) transmissionToPod(pkt *model.Packet, pod *v1.Pod, iif string) (model.Transmission, error) {
	if !pod.Spec.HostNetwork && pod.Spec.NodeName == h.nodeInfo.NodeName {
		// check local veth
		key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		if peerNetNS, ok := lo.Find(h.nodeInfo.SubNetNSInfo, func(info netstack.NetNSInfo) bool { return info.Key == key }); ok {
			h.net.AssertVEthPeerBridge("eth0", &peerNetNS, "cni0")
		}

		// send to local pod
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
			Type:            model.LinkVeth,
			Source:          h.netNode,
			Packet:          pktOut,
			SourceAttribute: model.SimpleLinkAttribute{Interface: h.bridge},
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

	node, err := h.ipCache.GetNodeFromName(pod.Spec.NodeName)
	if err != nil {
		return model.Transmission{}, err
	}

	return h.transmissionToNode(pkt, node, iif)
}

func (h *flannelHost) transmissionToNode(pkt *model.Packet, node *v1.Node, _ string) (model.Transmission, error) {
	pktOut := pkt.DeepCopy()

	err := h.masquerade(pktOut)
	if err != nil {
		return model.Transmission{}, err
	}

	err = h.checkRoute(pkt)
	if err != nil {
		return model.Transmission{}, err
	}

	err = h.route.AssertBackend(pktOut)
	if err != nil {
		return model.Transmission{}, err
	}

	out, err := h.route.Encap(pktOut)
	if err != nil {
		return model.Transmission{}, err
	}

	localNode, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return model.Transmission{}, err
	}

	if h.infraShim != nil {
		suspicions, err := h.infraShim.NodeToNode(localNode, h.iface, node, pktOut)
		if err != nil {
			return model.Transmission{}, err
		}
		h.netNode.Suspicions = append(h.netNode.Suspicions, suspicions...)
	}

	link := &model.Link{
		Type:            model.LinkInfra,
		Source:          h.netNode,
		Packet:          out,
		SourceAttribute: model.SimpleLinkAttribute{Interface: h.iface},
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

func (h *flannelHost) transmissionToExternal(pkt *model.Packet, _ string) (model.Transmission, error) {
	pktOut := pkt.DeepCopy()
	err := h.masquerade(pktOut)
	if err != nil {
		return model.Transmission{}, err
	}
	err = h.checkRoute(pktOut)
	if err != nil {
		return model.Transmission{}, err
	}

	node, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return model.Transmission{}, err
	}
	if h.infraShim != nil {
		suspicions, err := h.infraShim.NodeToExternal(node, h.iface, pktOut)
		if err != nil {
			return model.Transmission{}, err
		}
		h.netNode.Suspicions = append(h.netNode.Suspicions, suspicions...)
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
func (h *flannelHost) ToPod(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, pod *v1.Pod) ([]model.Transmission, error) {
	makeLink := func(pkt *model.Packet, iif string) (model.Transmission, error) {
		return h.transmissionToPod(pkt, pod, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *flannelHost) ToHost(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, node *v1.Node) ([]model.Transmission, error) {
	makeLink := func(pkt *model.Packet, iif string) (model.Transmission, error) {
		return h.transmissionToNode(pkt, node, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *flannelHost) ToService(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, service *v1.Service) ([]model.Transmission, error) {
	iif := h.detectIif(upstream)
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
			if err == netstack.ErrNoRouteToHost {
				h.netNode.AddSuspicion(model.SuspicionLevelFatal, fmt.Sprintf("no route to host: %v", dst))
				return nil, &assertions.CannotBuildTransmissionError{
					SrcNode: h.netNode,
					Err:     fmt.Errorf("no route to host: %v", dst),
				}
			}
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

func (h *flannelHost) ToExternal(upstream *model.Link, dst model.Endpoint, protocol model.Protocol) ([]model.Transmission, error) {
	makeLink := func(pkt *model.Packet, iif string) (model.Transmission, error) {
		return h.transmissionToExternal(pkt, iif)
	}
	return h.to(upstream, dst, protocol, makeLink)
}

func (h *flannelHost) to(upstream *model.Link, dst model.Endpoint, protocol model.Protocol, transmit transmissionFunc) ([]model.Transmission, error) {
	iif := h.detectIif(upstream)
	var action *model.Action
	var transmission model.Transmission
	var pkt *model.Packet
	var nfAssertionFunc func(pktIn model.Packet, pktOut []model.Packet, iif string)

	if upstream != nil {
		if upstream.Type == model.LinkVeth {
			// send from local pod, assert veth
			if attr, ok := upstream.SourceAttribute.(model.VEthLinkAttribute); ok {
				h.net.AssertVEthOnBridge(attr.PeerIndex, h.bridge)
			}
		}

		upstream.DestinationAttribute = model.SimpleLinkAttribute{Interface: iif}
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
		action = model.ActionForward(upstream, []*model.Link{transmission.Link})
	} else {
		pkt = &model.Packet{
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
		action = model.ActionSend([]*model.Link{transmission.Link})
	}

	pktOut := transmission.Link.Packet
	nfAssertionFunc(*pkt, []model.Packet{*pktOut}, iif)

	h.netNode.DoAction(action)
	return []model.Transmission{transmission}, nil
}

func (h *flannelHost) Serve(upstream *model.Link, dst model.Endpoint, protocol model.Protocol) ([]model.Transmission, error) {
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
		return h.ToPod(upstream, model.Endpoint{
			IP:   innerPacket.Dst.String(),
			Type: model.EndpointTypePod,
			Port: innerPacket.Dport,
		}, innerPacket.Protocol, pod)
	}

	upstream.DestinationAttribute = model.SimpleLinkAttribute{Interface: iif}

	err := h.route.Assert(h.net, ack(upstream.Packet))
	if err != nil {
		return nil, err
	}

	h.net.AssertNetfilterServe(*upstream.Packet, iif)
	h.net.AssertListen(net.ParseIP(dst.IP), dst.Port, protocol)
	h.netNode.DoAction(model.ActionServe(upstream))

	return nil, nil
}

func (h *flannelHost) detectIif(upstream *model.Link) string {
	if upstream == nil {
		return ""
	}

	if upstream.Type == model.LinkVeth {
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
	h.net.AssertNetDevice(h.iface, netstack.Interface{
		MTU:   1500,
		State: netstack.LinkUP,
	})
	h.net.AssertHostBridge(h.bridge)
	h.net.AssertSysctls(map[string]string{
		"net.bridge.bridge-nf-call-iptables": "1",
		"net.ipv4.ip_forward":                "1",
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", utils.ConvertNICNameInSysctls(h.bridge)): "1",
		fmt.Sprintf("net.ipv4.conf.%s.forwarding", utils.ConvertNICNameInSysctls(h.iface)):  "1",
	}, model.SuspicionLevelFatal)

	return nil
}

func (h *flannelHost) initRoute() error {
	interfaceDev, ok := lo.Find(h.nodeInfo.NetNS.Interfaces, func(i netstack.Interface) bool { return i.Name == h.iface })
	if !ok {
		return fmt.Errorf("can not find interface named %s", h.iface)
	}

	ip, mask := netstack.GetDefaultIPv4(&interfaceDev)
	cidr := net.IPNet{
		IP:   ip,
		Mask: mask,
	}

	routes := map[string]assertions.RouteAssertion{
		h.podCIDR.String(): {Dev: &h.bridge, Scope: utils.ToPointer(netstack.ScopeLink)},
		cidr.String():      {Dev: &h.iface, Scope: utils.ToPointer(netstack.ScopeLink)},
	}

	if h.gateway != nil && !h.gateway.IsUnspecified() {
		routes["0.0.0.0/0"] = assertions.RouteAssertion{
			Dev:   &h.iface,
			Scope: utils.ToPointer(netstack.ScopeUniverse),
			Gw:    &h.gateway,
		}
	}

	node, err := h.ipCache.GetNodeFromName(h.nodeInfo.NodeName)
	if err != nil {
		return err
	}

	h.route = newFlannelRoute(routes, h.podCIDR, h.clusterCIDR, h.net, node, h.ipCache, h.cniMode, h.iface)
	return nil
}

func (h *flannelHost) masquerade(pkt *model.Packet) error {
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

func (h *flannelHost) doMasquerade(pkt *model.Packet) error {
	ip, _, err := h.nodeInfo.Router.RouteSrc(pkt, "", "")
	if err != nil {
		return err
	}
	pkt.Src = net.ParseIP(ip)
	return nil
}

func (h *flannelHost) checkRoute(pkt *model.Packet) error {
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
