package aliyun

import (
	"fmt"
	"net"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/infra/aliyun"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

type aliyunInfraShim struct {
	cloudManager *aliyun.CloudManager
	vpcAssertion *vpcAssertion
}

func NewInfraShim(cloudManager *aliyun.CloudManager) (network.InfraShim, error) {
	vpcAssertion, err := newVPCAssertion(cloudManager)
	if err != nil {
		return nil, err
	}

	return &aliyunInfraShim{
		vpcAssertion: vpcAssertion,
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

func (s *aliyunInfraShim) getECSFromNode(node *v1.Node) (string, error) {
	providerIDs := strings.Split(node.Spec.ProviderID, ".")
	if len(providerIDs) != 2 {
		return "", fmt.Errorf("provider id %s does not match format <region>.<instance>", node.Spec.ProviderID)
	}
	return providerIDs[1], nil
}

func (s *aliyunInfraShim) isPrivate(ip net.IP) bool {
	// include private address and 100.64.0.0/10
	return ip.IsPrivate() || (ip[0] == 100 && ip[1]&0xc0 == 64)
}
