package aliyun

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	slb "github.com/alibabacloud-go/slb-20140515/v4/client"

	"github.com/alibaba/kubeskoop/pkg/skoop/infra/aliyun"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	ecs "github.com/alibabacloud-go/ecs-20140526/v2/client"
	vpc "github.com/alibabacloud-go/vpc-20160428/v2/client"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"k8s.io/klog/v2"
)

type securityPolicyVerdict string

const (
	securityPolicyVerdictAccept securityPolicyVerdict = "Accept"
	securityPolicyVerdictDrop   securityPolicyVerdict = "Drop"
)

type vpcAssertion struct {
	cloudManager *aliyun.CloudManager
}

func newVPCAssertion(cloudManager *aliyun.CloudManager) (*vpcAssertion, error) {
	if cloudManager == nil {
		return nil, fmt.Errorf("cloud manager must be provided")
	}

	return &vpcAssertion{
		cloudManager: cloudManager,
	}, nil
}

func (a *vpcAssertion) AssertSecurityGroup(srcECS, dstECS string, pkt *model.Packet) ([]model.Suspicion, error) {
	if srcECS == "" && dstECS == "" {
		return nil, nil
	}

	var suspicions []model.Suspicion

	if srcECS != "" && dstECS != "" {
		srcECSInfo, err := a.cloudManager.GetECSInfo(srcECS)
		if err != nil {
			return nil, err
		}
		dstECSInfo, err := a.cloudManager.GetECSInfo(dstECS)
		if err != nil {
			return nil, err
		}

		var srcECSSecurityGroup map[string]aliyun.SecurityGroupRule
		srcENI := a.getENIFromIP(srcECSInfo, pkt.Src.String())
		if srcENI != nil {
			srcECSSecurityGroup = srcENI.SecurityGroups
		} else {
			srcECSSecurityGroup = srcECSInfo.Network.SecurityGroups
		}

		var dstECSSecurityGroup map[string]aliyun.SecurityGroupRule
		dstENI := a.getENIFromIP(dstECSInfo, pkt.Dst.String())
		if dstENI != nil {
			dstECSSecurityGroup = dstENI.SecurityGroups
		} else {
			dstECSSecurityGroup = dstECSInfo.Network.SecurityGroups
		}

		sgIntersection := a.intersection(srcECSSecurityGroup, dstECSSecurityGroup)
		if len(sgIntersection) == 0 {
			suspicions = append(suspicions, model.Suspicion{
				Level: model.SuspicionLevelWarning,
				Message: fmt.Sprintf("%s(%s) and %s(%s) do not have same security group",
					srcECS, a.getECSIP(srcECSInfo), dstECS, a.getECSIP(dstECSInfo)),
			})
		}

		// eni 只能绑定一个类型sg
		// sg 相交场景，且相交的是普通安全组，默认策略为 Allow
		// sg 相交场景，且为企业安全组，默认策略 Deny
		// sg 不相交，默认 Deny
		sgPolicyVerdict := securityPolicyVerdictDrop
		if len(sgIntersection) > 0 {
			if sgIntersection[0].Type == aliyun.SecurityGroupTypeNormal {
				sgPolicyVerdict = securityPolicyVerdictAccept
			}

			checkResult, err := a.checkSourceOut(pkt, srcECSSecurityGroup)
			if err != nil {
				return nil, err
			}
			if !checkResult {
				sgIDs := lo.MapToSlice(srcECSSecurityGroup,
					func(id string, _ aliyun.SecurityGroupRule) string { return id })

				suspicions = append(suspicions, model.Suspicion{
					Level: model.SuspicionLevelFatal,
					Message: fmt.Sprintf("%s(%s) security group %v not allow packet to %s:%d",
						srcECS, a.getECSIP(srcECSInfo), sgIDs, pkt.Dst.String(), pkt.Dport),
				})
			}

			if !slices.Contains(srcECSInfo.Network.IP, pkt.Src.String()) || len(sgIntersection) == 0 ||
				(len(sgIntersection) > 0 && srcECSSecurityGroup[sgIntersection[0].ID].Type == "enterprise") {
				checkResult, err := a.checkDestinationIn(pkt, dstECSSecurityGroup, sgPolicyVerdict)
				if err != nil {
					return nil, err
				}
				if !checkResult {
					sgIDs := lo.MapToSlice(dstECSSecurityGroup,
						func(id string, _ aliyun.SecurityGroupRule) string { return id })
					suspicions = append(suspicions, model.Suspicion{
						Level: model.SuspicionLevelFatal,
						Message: fmt.Sprintf("%s(%s) security group %v not allow packet from %s to port %d",
							dstECS, a.getECSIP(dstECSInfo), sgIDs, pkt.Src.String(), pkt.Dport),
					})
				}
			}
		}
	} else if srcECS != "" && dstECS == "" {
		srcECSInfo, err := a.cloudManager.GetECSInfo(srcECS)
		if err != nil {
			return nil, err
		}

		srcENI := a.getENIFromIP(srcECSInfo, pkt.Src.String())
		var srcECSSecurityGroup map[string]aliyun.SecurityGroupRule
		if srcENI == nil {
			srcECSSecurityGroup = srcECSInfo.Network.SecurityGroups
		} else {
			srcECSSecurityGroup = srcENI.SecurityGroups
		}

		checkResult, err := a.checkSourceOut(pkt, srcECSSecurityGroup)
		if err != nil {
			return nil, err
		}

		if !checkResult {
			sgIDs := lo.Keys(srcECSSecurityGroup)
			suspicions = append(suspicions, model.Suspicion{
				Level: model.SuspicionLevelFatal,
				Message: fmt.Sprintf("%s(%s) security group %v not allow packet from to %s:%d",
					srcECS, a.getECSIP(srcECSInfo), sgIDs, pkt.Dst.String(), pkt.Dport),
			})
		}
	} else if srcECS == "" && dstECS != "" {
		dstECSInfo, err := a.cloudManager.GetECSInfo(dstECS)
		if err != nil {
			return nil, err
		}

		dstENI := a.getENIFromIP(dstECSInfo, pkt.Dst.String())
		var dstECSSecurityGroup map[string]aliyun.SecurityGroupRule
		if dstENI == nil {
			dstECSSecurityGroup = dstECSInfo.Network.SecurityGroups
		} else {
			dstECSSecurityGroup = dstENI.SecurityGroups
		}

		sgs := lo.Values(dstECSSecurityGroup)
		if len(sgs) > 0 {
			sgPolicyVerdict := securityPolicyVerdictDrop
			checkResult, err := a.checkDestinationIn(pkt, dstECSSecurityGroup, sgPolicyVerdict)
			if err != nil {
				return nil, err
			}
			if !checkResult {
				sgIDs := lo.MapToSlice(dstECSSecurityGroup,
					func(id string, _ aliyun.SecurityGroupRule) string { return id })
				suspicions = append(suspicions, model.Suspicion{
					Level: model.SuspicionLevelFatal,
					Message: fmt.Sprintf("%s(%s) security group %v not allow packet from %s to port %d",
						dstECS, a.getECSIP(dstECSInfo), sgIDs, pkt.Src.String(), pkt.Dport),
				})
			}
		}
	}

	return suspicions, nil
}

func (a *vpcAssertion) AssertRoute(srcECS, dstECS string, pkt *model.Packet, privateIP string) ([]model.Suspicion, error) {
	var suspicions []model.Suspicion
	var routeEntries []*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry
	var srcECSInfo, dstECSInfo *aliyun.ECSInfo
	var err error

	if srcECS == "" && dstECS == "" {
		return nil, nil
	}

	if srcECS != "" {
		srcECSInfo, err = a.cloudManager.GetECSInfo(srcECS)
		if err != nil {
			return nil, err
		}

		routeEntries = srcECSInfo.Network.RouteTableEntries
		if !slices.Contains(srcECSInfo.Network.IP, pkt.Src.String()) {
			routeEntries = srcECSInfo.Network.VpcDefaultRouteTableEntries
		}
	}

	if dstECS != "" {
		dstECSInfo, err = a.cloudManager.GetECSInfo(dstECS)
		if err != nil {
			return nil, err
		}

		if srcECS == "" {
			routeEntries = dstECSInfo.Network.VpcDefaultRouteTableEntries
		}
	}

	dstRouteEntry, err := routeMatchPacket(pkt.Dst.String(), routeEntries)
	if err != nil {
		return nil, err
	}
	if dstRouteEntry == nil {
		suspicions = append(suspicions, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("no route entry for destination ip %q", pkt.Dst),
		})
	}

	if dstECS != "" && dstRouteEntry != nil && !slices.Contains(dstECSInfo.Network.IP, pkt.Dst.String()) {
		// we do not route dst ip in ecs network ips
		// is there any situation that len(NextHop) == 0?
		nextHop := dstRouteEntry.NextHops.NextHop[0]
		if *nextHop.NextHopType != "local" &&
			!(*nextHop.NextHopType == "Instance" && *nextHop.NextHopId == dstECS) {
			suspicions = append(suspicions, model.Suspicion{
				Level: model.SuspicionLevelFatal,
				Message: fmt.Sprintf("error route next hop for destination ip \"%s\", expect: \"Instance-%s\", actually: \"%s-%s\"",
					pkt.Dst.String(), dstECS, *nextHop.NextHopType, *nextHop.NextHopId),
			})
		}
	}

	// reverse path
	if srcECS != "" {
		var srcRouteEntry *vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry
		if dstECS != "" {
			if !slices.Contains(dstECSInfo.Network.IP, pkt.Dst.String()) {
				routeEntries = dstECSInfo.Network.VpcDefaultRouteTableEntries
			} else {
				routeEntries = srcECSInfo.Network.RouteTableEntries
			}
		} else {
			routeEntries = srcECSInfo.Network.VpcDefaultRouteTableEntries
		}

		if privateIP != "" {
			srcRouteEntry, err = routeMatchPacket(privateIP, routeEntries)
			if err != nil {
				return nil, err
			}
		} else {
			srcRouteEntry, err = routeMatchPacket(pkt.Src.String(), routeEntries)
		}
		if err != nil {
			return nil, err
		}

		if srcRouteEntry == nil {
			suspicions = append(suspicions, model.Suspicion{
				Level:   model.SuspicionLevelFatal,
				Message: fmt.Sprintf("no route entry for src ip %q", pkt.Src.String()),
			})
		}

		if srcRouteEntry != nil && !slices.Contains(srcECSInfo.Network.IP, pkt.Src.String()) {
			nextHop := srcRouteEntry.NextHops.NextHop[0]
			if nextHop.NextHopRegionId != nil && *nextHop.NextHopRegionId != "local" &&
				!(*nextHop.NextHopType == "Instance" && *nextHop.NextHopId == srcECS) {
				suspicions = append(suspicions, model.Suspicion{
					Level: model.SuspicionLevelFatal,
					Message: fmt.Sprintf("error route next hop for source ip: %q, expect: \"Instance-%s\", actual: \"%s-%s\"",
						pkt.Src, srcECSInfo.ID, *nextHop.NextHopType, *nextHop.NextHopId),
				})
			}
		}

	}

	return suspicions, nil
}

func (a *vpcAssertion) AssertSNAT(srcECS string, pkt *model.Packet, privateIP string) ([]model.Suspicion, error) {
	var suspicions []model.Suspicion
	eniInfo, err := a.cloudManager.GetENIInfoFromVPCAndPrivateIP(a.cloudManager.VPC(), privateIP)
	if err != nil {
		return nil, err
	}

	vswitchID := *eniInfo.VSwitchId
	klog.V(3).Infof("vswitch id %s", vswitchID)

	nextHop, err := a.findNextHop(pkt, srcECS)
	if err != nil {
		return nil, err
	}

	if nextHop == nil {
		suspicions = append(suspicions, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("no next hop for destination ip %q", pkt.Dst),
		})
		return suspicions, nil
	}

	if *nextHop.NextHopType != "NatGateway" {
		suspicions = append(suspicions, model.Suspicion{
			Level: model.SuspicionLevelFatal,
			Message: fmt.Sprintf("expect next hop for destination ip %q to be NatGateway, but \"%s-%s\"",
				pkt.Dst, *nextHop.NextHopType, *nextHop.NextHopId),
		})
		return suspicions, nil
	}

	ngwID := *nextHop.NextHopId
	snatEntry, err := a.findSNATEntry(pkt, ngwID, vswitchID)
	if err != nil {
		return nil, err
	}

	if snatEntry == nil {
		suspicions = append(suspicions, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("no snat entry on nat gateway %q for destination ip %q", ngwID, pkt.Dst),
		})
	}

	return suspicions, nil
}

func (a *vpcAssertion) getENIFromIP(ecsInfo *aliyun.ECSInfo, dstIP string) *aliyun.ENIInfo {
	for _, eni := range ecsInfo.Network.NetworkInterfaces {
		for _, privateIP := range eni.NetworkInterfaceSet.PrivateIpSets.PrivateIpSet {
			if *privateIP.PrivateIpAddress == dstIP {
				return eni
			}
		}
	}
	return nil
}

func (a *vpcAssertion) intersection(sga, sgb map[string]aliyun.SecurityGroupRule) []aliyun.SecurityGroupRule {
	existed := map[string]struct{}{}
	for _, sg := range sga {
		existed[sg.ID] = struct{}{}
	}

	var ret []aliyun.SecurityGroupRule
	for _, sg := range sgb {
		if _, ok := existed[sg.ID]; ok {
			ret = append(ret, sg)
		}
	}

	return ret
}

func (a *vpcAssertion) getECSIP(ecs *aliyun.ECSInfo) string {
	if len(ecs.Network.IP) == 0 {
		return "!NOT FOUND!"
	}

	return ecs.Network.IP[0]
}

func (a *vpcAssertion) checkSourceOut(pkt *model.Packet, sgs map[string]aliyun.SecurityGroupRule) (bool, error) {
	hasEnterpriseSg := false
	var outRules []*ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission
	for _, sg := range sgs {
		if sg.Type == "enterprise" {
			hasEnterpriseSg = true
		}

		outRules = append(outRules, sg.OutRule.Allows...)
		outRules = append(outRules, sg.OutRule.Drops...)
	}

	if hasEnterpriseSg {
		return a.packetPassRules(pkt, outRules, securityPolicyVerdictDrop)
	}
	return a.packetPassRules(pkt, outRules, securityPolicyVerdictAccept)
}

func (a *vpcAssertion) checkDestinationIn(pkt *model.Packet, sgs map[string]aliyun.SecurityGroupRule, defaultPolicy securityPolicyVerdict) (bool, error) {
	var inRules []*ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission
	for _, sg := range sgs {
		inRules = append(inRules, sg.InRule.Allows...)
		inRules = append(inRules, sg.InRule.Drops...)
	}

	if defaultPolicy == "" {
		defaultPolicy = securityPolicyVerdictDrop
	}

	return a.packetPassRules(pkt, inRules, defaultPolicy)
}

func (a *vpcAssertion) packetPassRules(pkt *model.Packet, outRules []*ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission, defaultPolicy securityPolicyVerdict) (bool, error) {
	var filteredRules []*ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission
	for _, rule := range outRules {
		match, err := ruleMatchPacket(pkt, rule)
		if err != nil {
			return false, err
		}

		if match {
			filteredRules = append(filteredRules, rule)
		}
	}

	sortSecurityGroupRules(filteredRules)
	if len(filteredRules) > 0 {
		return *filteredRules[0].Policy == string(securityPolicyVerdictAccept), nil
	}

	klog.V(3).Infof("No rule match for packet, default policy for SecurityGroup is %q", defaultPolicy)
	return defaultPolicy == securityPolicyVerdictAccept, nil
}

func (a *vpcAssertion) findNextHop(pkt *model.Packet, srcECS string) (*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntryNextHopsNextHop, error) {
	ecsInfo, err := a.cloudManager.GetECSInfo(srcECS)
	if err != nil {
		return nil, err
	}

	routeEntries := ecsInfo.Network.RouteTableEntries
	if !slices.Contains(ecsInfo.Network.IP, pkt.Src.String()) {
		routeEntries = ecsInfo.Network.VpcDefaultRouteTableEntries
	}

	route, err := routeMatchPacket(pkt.Dst.String(), routeEntries)
	if err != nil {
		return nil, err
	}

	return route.NextHops.NextHop[0], nil
}

func (a *vpcAssertion) findSNATEntry(pkt *model.Packet, ngwID, vswitchID string) (*vpc.DescribeSnatTableEntriesResponseBodySnatTableEntriesSnatTableEntry, error) {
	vswitch, err := a.cloudManager.GetVSwitch(vswitchID)
	if err != nil {
		return nil, err
	}

	ngw, err := a.cloudManager.GetNatGatewayInfo(ngwID)
	if err != nil {
		return nil, err
	}

	snatTables := ngw.SnatTableIds.SnatTableId
	var defaultEntry *vpc.DescribeSnatTableEntriesResponseBodySnatTableEntriesSnatTableEntry
	for _, t := range snatTables {
		snatEntries, err := a.cloudManager.GetSNATEntriesBySegment(*t, fmt.Sprintf("%s/32", pkt.Src))
		if err != nil {
			return nil, err
		}
		if len(snatEntries) > 0 {
			defaultEntry = snatEntries[0]
			break
		}

		snatEntries, err = a.cloudManager.GetSNATEntriesBySegment(*t, *vswitch.CidrBlock)
		if err != nil {
			return nil, err
		}
		if len(snatEntries) > 0 {
			defaultEntry = snatEntries[0]
		}
	}

	return defaultEntry, nil
}

type slbAssertion struct {
	cloudManager *aliyun.CloudManager
}

func newSLBAssertion(cloudManager *aliyun.CloudManager) (*slbAssertion, error) {
	return &slbAssertion{
		cloudManager: cloudManager,
	}, nil
}

func (a *slbAssertion) assertLoadBalancer(lb *slb.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancer, dport uint16, protocol model.Protocol) ([]model.Suspicion, error) {
	var sus []model.Suspicion
	if lb.LoadBalancerStatus != nil && *lb.LoadBalancerStatus != "active" {
		sus = append(sus, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("loadBalancer(%s) status is %q, not \"active\"", *lb.LoadBalancerId, *lb.LoadBalancerStatus),
		})
	}

	status, err := a.cloudManager.GetSLBHealthStatus(*lb.LoadBalancerId, int32(dport), string(protocol))
	if err != nil {
		return nil, err
	}

	if len(status) == 0 {
		return sus, nil
	}

	s := status[0]
	if s.ServerHealthStatus != nil && *s.ServerHealthStatus != "normal" {
		sus = append(sus, model.Suspicion{
			Level:   model.SuspicionLevelWarning,
			Message: fmt.Sprintf("backend %s health status for port %d is %q, not \"normal\"", *s.ServerIp, *s.Port, *s.ServerHealthStatus),
		})
	}

	return sus, nil
}

func (a *slbAssertion) assertListenerAndServerGroup(lbID string, port int32, protocol string, packet *model.Packet, backends []network.LoadBalancerBackend) ([]model.Suspicion, error) {
	var sus []model.Suspicion
	lis, err := a.cloudManager.GetSLBListener(lbID, port, protocol)
	if err != nil {
		return nil, err
	}

	if lis == nil {
		sus = append(sus, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("cannot find listener port %d protocol %q for slb %q", port, protocol, lbID),
		})
		return sus, nil
	}

	if lis.TCP != nil {
		if lis.TCP.Status != nil && *lis.TCP.Status != "running" {
			sus = append(sus, model.Suspicion{
				Level: model.SuspicionLevelFatal,
				Message: fmt.Sprintf("status of loadbalancer %q for tcp port %d is %q, not \"running\"",
					lbID, port, *lis.TCP.Status),
			})
		}

		if lis.TCP.AclStatus != nil && *lis.TCP.AclStatus == "on" && lis.TCP.AclId != nil {
			s, err := a.assertACL(*lis.TCP.AclId, *lis.TCP.AclType, packet)
			if err != nil {
				return nil, err
			}
			sus = append(sus, s...)
		}

		if lis.TCP.VServerGroupId != nil && *lis.TCP.VServerGroupId != "" {
			s, err := a.assertServerGroup(*lis.TCP.VServerGroupId, backends)
			if err != nil {
				return nil, err
			}
			sus = append(sus, s...)
		} else {
			sus = append(sus, model.Suspicion{
				Level:   model.SuspicionLevelFatal,
				Message: fmt.Sprintf("cannot find vserver group of loadbalancer %q port %d", lbID, port),
			})
		}
		return sus, nil
	}

	if lis.UDP != nil {
		if lis.UDP.Status != nil && *lis.UDP.Status != "running" {
			sus = append(sus, model.Suspicion{
				Level: model.SuspicionLevelFatal,
				Message: fmt.Sprintf("status of loadbalancer %q for udp port %d is %q, not \"running\"",
					lbID, port, *lis.TCP.Status),
			})
		}

		if lis.UDP.AclStatus != nil && *lis.UDP.AclStatus == "on" && lis.TCP.AclId != nil {
			s, err := a.assertACL(*lis.UDP.AclId, *lis.UDP.AclType, packet)
			if err != nil {
				return nil, err
			}
			sus = append(sus, s...)
		}

		if lis.UDP.VServerGroupId != nil && *lis.UDP.VServerGroupId != "" {
			s, err := a.assertServerGroup(*lis.UDP.VServerGroupId, backends)
			if err != nil {
				return nil, err
			}
			sus = append(sus, s...)
		} else {
			sus = append(sus, model.Suspicion{
				Level:   model.SuspicionLevelFatal,
				Message: fmt.Sprintf("cannot find vserver group of loadbalancer %q port %d", lbID, port),
			})
		}
		return sus, nil
	}

	return nil, fmt.Errorf("not supported protocol %s", protocol)
}

func (a *slbAssertion) assertACL(aclID, t string, packet *model.Packet) ([]model.Suspicion, error) {
	acl, err := a.cloudManager.GetACLFromID(aclID)
	if err != nil {
		return nil, err
	}
	if acl == nil {
		return nil, fmt.Errorf("cannot find acl %q", aclID)
	}

	// When there is no entries in an ACL, it will allow all packets to pass.
	if len(acl.AclEntrys.AclEntry) == 0 {
		return nil, nil
	}

	ipInACL := lo.ContainsBy(acl.AclEntrys.AclEntry, func(i *slb.DescribeAccessControlListAttributeResponseBodyAclEntrysAclEntry) bool {
		if i == nil || i.AclEntryIP == nil {
			return false
		}
		_, ipCIDR, _ := parseIPOrCIDR(*i.AclEntryIP)
		return ipCIDR.Contains(packet.Src)
	})

	var sus []model.Suspicion
	if (t == "white" && !ipInACL) || (t == "black" && ipInACL) {
		sus = append(sus, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("acl %s type %s on loadbalancer does not allow packet to pass", aclID, t),
		})
	}

	return sus, nil
}

func (a *slbAssertion) assertServerGroup(sgID string, backends []network.LoadBalancerBackend) ([]model.Suspicion, error) {
	sg, err := a.cloudManager.GetSLBVserverGroup(sgID)
	if err != nil {
		return nil, err
	}
	if sg == nil {
		return nil, fmt.Errorf("cannot find vserver group %q", sgID)
	}

	var sus []model.Suspicion
	if len(sg.BackendServers.BackendServer) == 0 {
		sus = append(sus, model.Suspicion{
			Level:   model.SuspicionLevelFatal,
			Message: fmt.Sprintf("server group %q has no backend", sgID),
		})
	}

	if len(sg.BackendServers.BackendServer) != len(backends) {
		sus = append(sus, model.Suspicion{
			Level: model.SuspicionLevelWarning,
			Message: fmt.Sprintf("backend count %d is not equal to expected backend count %d",
				len(sg.BackendServers.BackendServer), len(backends)),
		})
	}

	for _, s := range sg.BackendServers.BackendServer {
		contains := lo.ContainsBy(backends, func(b network.LoadBalancerBackend) bool {
			if *s.Port != int32(b.Port) {
				return false
			}
			if s.ServerIp != nil && b.IP != "" {
				return *s.ServerIp == b.IP
			}
			if s.ServerId != nil && b.ID != "" {
				return *s.ServerId == b.ID
			}
			// If we're unable to find backend by the given information,
			// return true to prevent suspicions.
			return true
		})

		if !contains {
			backend := ""
			if s.ServerIp != nil {
				backend = fmt.Sprintf("%s:%d", *s.ServerIp, *s.Port)
			} else if s.ServerId != nil {
				backend = fmt.Sprintf("%s:%d", *s.ServerId, *s.Port)
			} else {
				backend = fmt.Sprintf("(unknown):%d", *s.Port)
			}

			sus = append(sus, model.Suspicion{
				Level: model.SuspicionLevelWarning,
				Message: fmt.Sprintf("backend %q in server group %q is unexpected",
					backend, sgID),
			})
		}
		// limit suspicions count to not exceed 5
		if len(sus) > 5 {
			break
		}
	}

	return sus, nil
}

func ruleMatchPacket(pkt *model.Packet, rule *ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission) (bool, error) {
	if rule.DestCidrIp != nil && *rule.DestCidrIp != "" {
		_, dstCidrIP, err := net.ParseCIDR(*rule.DestCidrIp)
		if err != nil {
			return false, err
		}

		if dstCidrIP.Contains(pkt.Dst) {
			if *rule.IpProtocol == "ALL" || strings.EqualFold(string(pkt.Protocol), *rule.IpProtocol) {
				if pkt.Dport == 0 {
					return true, nil
				}

				portRange := strings.Split(*rule.PortRange, "/")
				// assert len(portRange) == 2
				if portRange[0] == "-1" && portRange[1] == "-1" {
					return true, nil
				}

				pStart, err := strconv.Atoi(portRange[0])
				if err != nil {
					return false, err
				}
				pEnd, err := strconv.Atoi(portRange[1])
				if err != nil {
					return false, nil
				}

				if pStart <= int(pkt.Dport) && pEnd >= int(pkt.Dport) {
					return true, nil
				}
			}
		}
	}

	if rule.SourceCidrIp != nil && *rule.SourceCidrIp != "" {
		_, srcCidrIP, err := net.ParseCIDR(*rule.SourceCidrIp)
		if err != nil {
			return false, err
		}

		if srcCidrIP.Contains(pkt.Src) {
			if *rule.IpProtocol == "ALL" || strings.EqualFold(string(pkt.Protocol), *rule.IpProtocol) {
				if pkt.Dport == 0 {
					return true, nil
				}

				portRange := strings.Split(*rule.PortRange, "/")
				// assert len(portRange) == 2
				if portRange[0] == "-1" && portRange[1] == "-1" {
					return true, nil
				}

				pStart, err := strconv.Atoi(portRange[0])
				if err != nil {
					return false, err
				}
				pEnd, err := strconv.Atoi(portRange[1])
				if err != nil {
					return false, err
				}

				if pStart <= int(pkt.Dport) && pEnd >= int(pkt.Dport) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func sortSecurityGroupRules(sgs []*ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission) {
	slices.SortStableFunc(sgs, func(a, b *ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission) bool {
		portRangeA := strings.Split(*a.PortRange, "/")
		pStartA, _ := strconv.Atoi(portRangeA[0])
		pEndA, _ := strconv.Atoi(portRangeA[1])
		if pStartA == -1 && pEndA == -1 {
			pStartA, pEndA = 0, 65535
		}

		portRangeB := strings.Split(*b.PortRange, "/")
		pStartB, _ := strconv.Atoi(portRangeB[0])
		pEndB, _ := strconv.Atoi(portRangeB[1])
		if pStartB == -1 && pEndB == -1 {
			pStartB, pEndB = 0, 65535
		}

		if *a.Priority != *b.Priority {
			return *a.Priority < *b.Priority
		}

		if (a.SourceCidrIp != nil && *a.SourceCidrIp != "") || (b.SourceCidrIp != nil && *b.SourceCidrIp != "") {
			if (a.SourceCidrIp == nil || *a.SourceCidrIp == "") || (b.SourceCidrIp == nil || *b.SourceCidrIp == "") {
				return a.SourceCidrIp != nil && *a.SourceCidrIp != ""
			}

			_, netA, _ := net.ParseCIDR(*a.SourceCidrIp)
			onesA, _ := netA.Mask.Size()

			_, netB, _ := net.ParseCIDR(*a.SourceCidrIp)
			onesB, _ := netB.Mask.Size()

			if onesA != onesB {
				return onesA > onesB
			}
		}

		if (a.DestCidrIp != nil && *a.DestCidrIp != "") || (b.DestCidrIp != nil && *b.DestCidrIp != "") {
			if (a.DestCidrIp == nil || *a.DestCidrIp == "") || (b.DestCidrIp == nil || *b.DestCidrIp == "") {
				return a.DestCidrIp != nil && *a.DestCidrIp != ""
			}

			_, netA, _ := net.ParseCIDR(*a.DestCidrIp)
			onesA, _ := netA.Mask.Size()

			_, netB, _ := net.ParseCIDR(*a.DestCidrIp)
			onesB, _ := netB.Mask.Size()

			if onesA != onesB {
				return onesA > onesB
			}
		}

		if *a.Policy != *b.Policy {
			return *a.Policy == string(securityPolicyVerdictDrop)
		}

		return (pEndA - pStartA) < (pEndB - pStartB)
	})
}

func routeMatchPacket(ip string, routes []*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry) (*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry, error) {
	type routesAndCIDR struct {
		entry *vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry
		cidr  *net.IPNet
	}

	var filteredRoutes []routesAndCIDR

	netIP := net.ParseIP(ip)
	for _, r := range routes {
		_, cidr, err := net.ParseCIDR(*r.DestinationCidrBlock)
		if err != nil {
			return nil, fmt.Errorf("parse route table %q dstination cidr error: %s", *r.RouteTableId, err)
		}

		if cidr.Contains(netIP) {
			filteredRoutes = append(filteredRoutes, routesAndCIDR{entry: r, cidr: cidr})
		}
	}

	slices.SortFunc(filteredRoutes, func(a, b routesAndCIDR) bool {
		onesA, _ := a.cidr.Mask.Size()
		onesB, _ := b.cidr.Mask.Size()
		return onesA > onesB
	})

	if len(filteredRoutes) > 0 {
		return filteredRoutes[0].entry, nil
	}

	return nil, nil
}

func parseIPOrCIDR(s string) (net.IP, *net.IPNet, error) {
	ip, cidr, err := net.ParseCIDR(s)
	if err == nil {
		return ip, cidr, err
	}

	return net.ParseCIDR(fmt.Sprintf("%s/32", s))
}
