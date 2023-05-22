package aliyun

import (
	"fmt"
	"net"
	"strings"

	slb "github.com/alibabacloud-go/slb-20140515/v4/client"
	"github.com/samber/lo"

	"github.com/alibaba/kubeskoop/pkg/skoop/infra/aliyun"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type aliyunInfraShim struct {
	cloudManager *aliyun.CloudManager
	vpcAssertion *vpcAssertion
	slbAssertion *slbAssertion
}

func NewInfraShim(cloudManager *aliyun.CloudManager) (network.InfraShim, error) {
	vpcAssertion, err := newVPCAssertion(cloudManager)
	if err != nil {
		return nil, err
	}

	slbAssertion, err := newSLBAssertion(cloudManager)
	if err != nil {
		return nil, err
	}

	return &aliyunInfraShim{
		vpcAssertion: vpcAssertion,
		slbAssertion: slbAssertion,
		cloudManager: cloudManager,
	}, nil
}

func (s *aliyunInfraShim) NodeToNode(src *v1.Node, _ string, dst *v1.Node, packet *model.Packet) ([]model.Suspicion, error) {
	klog.V(3).Infof("Node to node packet %+v", packet)
	srcID, err := s.getECSFromNode(src)
	if err != nil {
		return nil, err
	}

	dstID, err := s.getECSFromNode(dst)
	if err != nil {
		return nil, err
	}

	suspicions, err := s.vpcAssertion.AssertSecurityGroup(srcID, dstID, packet)
	if err != nil {
		return nil, err
	}

	routeSuspicions, err := s.vpcAssertion.AssertRoute(srcID, dstID, packet, "")
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	suspicions = append(suspicions, routeSuspicions...)

	return suspicions, nil
}

func (s *aliyunInfraShim) NodeToExternal(src *v1.Node, _ string, packet *model.Packet) ([]model.Suspicion, error) {
	srcID, err := s.getECSFromNode(src)
	if err != nil {
		return nil, err
	}
	ecsInfo, err := s.cloudManager.GetECSInfo(srcID)
	if err != nil {
		return nil, err
	}

	if s.isPrivate(packet.Dst) {
		return s.vpcAssertion.AssertSecurityGroup(srcID, "", packet)
	}

	var suspicions []model.Suspicion
	if ecsInfo.Network.EIPAddress != "" {
		klog.V(3).Infof("Use eip address for ecs %q, packet %q", srcID, packet)
		packet.Src = net.ParseIP(ecsInfo.Network.EIPAddress)
		suspicions, err = s.vpcAssertion.AssertSecurityGroup(srcID, "", packet)
		if err != nil {
			return nil, err
		}
	} else {
		klog.V(3).Infof("Use nat gateway for ecs %q, packet %q", srcID, packet)
		suspicions, err = s.vpcAssertion.AssertSecurityGroup(srcID, "", packet)
		if err != nil {
			return nil, err
		}

		snatSuspicions, err := s.vpcAssertion.AssertSNAT(srcID, packet, ecsInfo.Network.IP[0])
		if err != nil {
			return nil, err
		}

		suspicions = append(suspicions, snatSuspicions...)
	}

	return suspicions, nil
}

func (s *aliyunInfraShim) ExternalToNode(dst *v1.Node, packet *model.Packet) ([]model.Suspicion, error) {
	dstID, err := s.getECSFromNode(dst)
	if err != nil {
		return nil, err
	}
	ecsInfo, err := s.cloudManager.GetECSInfo(dstID)
	if err != nil {
		return nil, err
	}

	suspicions, err := s.vpcAssertion.AssertSecurityGroup("", dstID, packet)
	if err != nil {
		return nil, err
	}
	if ecsInfo.Network.EIPAddress != "" && ecsInfo.Network.EIPAddress == packet.Dst.String() {
		return suspicions, nil
	}

	routeSuspicions, err := s.vpcAssertion.AssertRoute("", dstID, packet, "")
	if err != nil {
		return nil, err
	}
	suspicions = append(suspicions, routeSuspicions...)

	return suspicions, nil
}

func (s *aliyunInfraShim) ExternalToLoadBalancer(dst *v1.Service, packet *model.Packet, backends []network.LoadBalancerBackend) ([]model.Suspicion, error) {
	var sus []model.Suspicion

	lb, err := s.getCLBIDFromService(dst)
	if err != nil {
		return nil, err
	}

	lbSuspicions, err := s.slbAssertion.assertLoadBalancer(lb, packet.Dport, packet.Protocol)
	if err != nil {
		return nil, err
	}

	sus = append(sus, lbSuspicions...)

	// todo: support protocol set by annotation
	// todo: support named port
	p, found := lo.Find(dst.Spec.Ports, func(i v1.ServicePort) bool {
		return i.Port == int32(packet.Dport) && strings.EqualFold(string(i.Protocol), string(packet.Protocol))
	})

	if !found {
		return nil, fmt.Errorf("cannot find port %d protocol %s on service", packet.Dport, packet.Protocol)
	}

	protocol := strings.ToLower(string(p.Protocol))
	portSuspicions, err := s.slbAssertion.assertListenerAndServerGroup(*lb.LoadBalancerId,
		p.Port, protocol, packet, backends)
	if err != nil {
		return nil, err
	}
	sus = append(sus, portSuspicions...)

	return sus, nil
}

func (s *aliyunInfraShim) getECSFromNode(node *v1.Node) (string, error) {
	providerIDs := strings.Split(node.Spec.ProviderID, ".")
	if len(providerIDs) != 2 {
		return "", fmt.Errorf("provider id %s does not match format <region>.<instance>", node.Spec.ProviderID)
	}
	return providerIDs[1], nil
}

func (s *aliyunInfraShim) getCLBIDFromService(svc *v1.Service) (*slb.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancer, error) {
	if id, ok := svc.Labels["service.k8s.alibaba/loadbalancer-id"]; ok {
		lb, err := s.cloudManager.GetSLBFromID(id)
		if err != nil {
			return nil, err
		}
		if lb == nil {
			return nil, fmt.Errorf("cannot find loadbalacner by id %q", id)
		}
		return lb, nil
	}
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return nil, fmt.Errorf("cannot find loadbalancer ip on service %s/%s", svc.Namespace, svc.Name)
	}

	ip := net.ParseIP(svc.Status.LoadBalancer.Ingress[0].IP)
	if ip == nil {
		return nil, fmt.Errorf("ip %q on service %s/%s is invalid",
			svc.Status.LoadBalancer.Ingress[0].IP, svc.Namespace, svc.Name)
	}

	var lb *slb.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancer
	var err error
	if s.isPrivate(ip) {
		lb, err = s.cloudManager.GetSLBFromPrivateIP(ip.String())
	} else {
		lb, err = s.cloudManager.GetSLBFromPublicIP(ip.String())
	}
	if lb == nil {
		return nil, fmt.Errorf("cannot find loadbalancer by ip %q", ip)
	}
	if err != nil {
		return nil, err
	}
	return lb, nil
}

func (s *aliyunInfraShim) isPrivate(ip net.IP) bool {
	// vpc reserved address 100.64.0.0/10
	if ip[0] == 100 && ip[1]&0xc0 == 64 {
		return true
	}
	for _, cidr := range s.cloudManager.VPCCIDRs() {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
