package service

import (
	goctx "context"
	"fmt"
	"net"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/context"
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"

	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Backend struct {
	IP         string
	Port       uint16
	Masquerade bool
}

type Processor interface {
	//验证backends是否符合预期
	Validate(packet model2.Packet, backends []Backend, netns netstack.NetNS) error

	//根据packet和svc，返回正确的backends
	Process(packet model2.Packet, svc *v1.Service) []Backend
}

type KubeProxyServiceProcessor struct {
	mode        string
	clusterCIDR *net.IPNet
	client      *kubernetes.Clientset
}

func (k *KubeProxyServiceProcessor) Validate(packet model2.Packet, backends []Backend, netns netstack.NetNS) error {
	if k.mode != "ipvs" {
		return nil
	}
	if netns.IPVS == nil {
		log.Errorf("ipvs field in netns is nil which is not expected")
		return nil
	}
	ipvsService := netns.IPVS.GetService(packet.Protocol, packet.Dst.String(), packet.Dport)
	if ipvsService == nil {
		return fmt.Errorf("service has not been connfigured in ipvs")
	}

	backendsSet := make(map[string]bool)

	for _, backend := range backends {
		key := fmt.Sprintf("%s:%d", backend.IP, backend.Port)
		backendsSet[key] = true
	}

	var invalids []string
	for _, rs := range ipvsService.RS {
		key := fmt.Sprintf("%s:%d", rs.IP, rs.Port)
		if _, ok := backendsSet[key]; !ok {
			invalids = append(invalids, key)
		}
		delete(backendsSet, key)
	}

	if len(invalids) > 0 {
		return fmt.Errorf("ipvs realserver %s is not a valid k8s service backend, which could make network issues", strings.Join(invalids, ","))
	}

	if len(backendsSet) != 0 {
		return fmt.Errorf("k8s endpoint %s is not in ipvs realserver, which cloud make network issues", strings.Join(maps.Keys(backendsSet), ","))
	}

	return nil
}

func serviceTargetPort(svc *v1.Service, dport uint16, protocol model2.Protocol) uint16 {
	for _, port := range svc.Spec.Ports {
		if port.Port == int32(dport) && strings.EqualFold(string(port.Protocol), string(protocol)) {
			//TODO 处理named port
			if port.TargetPort.Type == intstr.String {
				klog.Warningf("named port not support now for service %q port %q", svc.Name, port.TargetPort.StrVal)
			}
			return uint16(port.TargetPort.IntVal)
		}
	}
	return 0
}

func serviceTargetPortByNodePort(svc *v1.Service, nodePort uint16, protocol model2.Protocol) uint16 {
	for _, port := range svc.Spec.Ports {
		if strings.EqualFold(string(port.Protocol), string(protocol)) && port.NodePort == int32(nodePort) {
			return uint16(port.NodePort)

		}
	}

	return 0
}

func serviceLBIPs(svc *v1.Service) []string {
	if svc.Spec.Type != "LoadBalancer" {
		return nil
	}
	var ret []string
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		ret = append(ret, ingress.IP)
	}
	return ret
}

func isTrafficLocalService(svc *v1.Service) bool {
	return svc.Spec.ExternalTrafficPolicy == v1.ServiceExternalTrafficPolicyTypeLocal
}

func (k *KubeProxyServiceProcessor) shouldMasquerade(packet model2.Packet, svc *v1.Service) (bool, uint16) {
	masquerade := false
	targetPort := serviceTargetPort(svc, packet.Dport, packet.Protocol)
	dst := packet.Dst.String()
	if targetPort != 0 && slices.Contains(serviceLBIPs(svc), dst) {
		masquerade = !isTrafficLocalService(svc)
	} else if targetPort != 0 && dst == svc.Spec.ClusterIP && k.clusterCIDR != nil {
		masquerade = !k.clusterCIDR.Contains(packet.Dst)
	} else {
		targetPortByNodePort := serviceTargetPortByNodePort(svc, packet.Dport, packet.Protocol)
		if targetPortByNodePort != 0 {
			targetPort = targetPortByNodePort
			masquerade = !isTrafficLocalService(svc)
		} else if slices.Contains(svc.Spec.ExternalIPs, dst) {
			masquerade = !isTrafficLocalService(svc)
		}
	}
	return masquerade, targetPort
}

func (k *KubeProxyServiceProcessor) Process(packet model2.Packet, svc *v1.Service) []Backend {
	masquerade, targetPort := k.shouldMasquerade(packet, svc)
	ep, err := k.client.CoreV1().Endpoints(svc.Namespace).Get(goctx.TODO(), svc.Name, metav1.GetOptions{})
	if err != nil {
		log.Errorf("error list endponts for service")
		return nil
	}

	var ret []Backend

	for _, ss := range ep.Subsets {
		for _, addr := range ss.Addresses {
			backend := Backend{
				IP:         addr.IP,
				Port:       targetPort,
				Masquerade: masquerade,
			}
			ret = append(ret, backend)
		}
	}

	return ret
}

func NewKubeProxyServiceProcessor(ctx *context.Context) *KubeProxyServiceProcessor {
	return &KubeProxyServiceProcessor{
		mode:        ctx.ClusterConfig().ProxyMode,
		clusterCIDR: ctx.ClusterConfig().ClusterCIDR,
		client:      ctx.KubernetesClient(),
	}
}

var _ Processor = &KubeProxyServiceProcessor{}
