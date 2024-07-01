package aliyun

import (
	"fmt"
	"net"
	"strings"
	"sync"

	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	openapiv2 "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ecs "github.com/alibabacloud-go/ecs-20140526/v2/client"
	slb "github.com/alibabacloud-go/slb-20140515/v4/client"
	"github.com/alibabacloud-go/tea/tea"
	vpc "github.com/alibabacloud-go/vpc-20160428/v2/client"
)

type CloudManager struct {
	vpcID    string
	region   string
	vpcCIDRs []*net.IPNet

	vpc *vpc.Client
	ecs *ecs.Client
	slb *slb.Client

	ecsInfoCache map[string]*ECSInfo
}

type CloudManagerOptions struct {
	Region            string
	AccessKeyID       string
	AccessKeySecret   string
	SecurityToken     string
	VPCID             string
	InstanceOfCluster string
}

type SecurityGroupType string
type SecurityGroupPolicy string

const (
	SecurityGroupTypeEnterprise SecurityGroupType = "enterprise"
	SecurityGroupTypeNormal     SecurityGroupType = "normal"

	SecurityGroupPolicyAccept SecurityGroupPolicy = "accept"
	SecurityGroupPolicyDrop   SecurityGroupPolicy = "drop"
)

type SecurityGroupRules struct {
	Allows []*ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission
	Drops  []*ecs.DescribeSecurityGroupAttributeResponseBodyPermissionsPermission
}

type SecurityGroupRule struct {
	ID      string
	Type    SecurityGroupType
	InRule  SecurityGroupRules
	OutRule SecurityGroupRules
}

type ENIInfo struct {
	NetworkInterfaceSet *ecs.DescribeNetworkInterfacesResponseBodyNetworkInterfaceSetsNetworkInterfaceSet
	RouteTableEntries   []*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry
	SecurityGroups      map[string]SecurityGroupRule
}

type ECSNetwork struct {
	IP                          []string
	VSwitchID                   string
	VpcID                       string
	SecurityGroups              map[string]SecurityGroupRule
	RouteTableEntries           []*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry
	VpcDefaultRouteTableEntries []*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry
	NetworkInterfaces           []*ENIInfo
	EIPAddress                  string
}

type ECSInfo struct {
	ID      string
	Status  string
	Network ECSNetwork
}

type Listener struct {
	TCP *slb.DescribeLoadBalancerTCPListenerAttributeResponseBody
	UDP *slb.DescribeLoadBalancerUDPListenerAttributeResponseBody
}

func NewCloudManager(options *CloudManagerOptions) (*CloudManager, error) {
	//TODO user-agent
	cfg := &openapiv2.Config{
		AccessKeyId:     tea.String(options.AccessKeyID),
		AccessKeySecret: tea.String(options.AccessKeySecret),
		SecurityToken:   tea.String(options.SecurityToken),
		RegionId:        tea.String(options.Region),
	}

	vpcClient, err := vpc.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create vpc client: %s", err)
	}

	ecsClient, err := ecs.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(options.AccessKeyID),
		AccessKeySecret: tea.String(options.AccessKeySecret),
		SecurityToken:   tea.String(options.SecurityToken),
		RegionId:        tea.String(options.Region),
	})
	if err != nil {
		return nil, fmt.Errorf("create ecs client: %s", err)
	}

	slbClient, err := slb.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create slb client: %s", err)
	}

	vpcID := ""
	if options.VPCID != "" {
		vpcID = options.VPCID
	} else if options.InstanceOfCluster != "" {
		request := &ecs.DescribeInstancesRequest{}
		request.SetRegionId(options.Region).
			SetInstanceIds(fmt.Sprintf("[\"%s\"]", options.InstanceOfCluster))

		response, err := ecsClient.DescribeInstances(request)
		if err != nil {
			return nil, err
		}

		if len(response.Body.Instances.Instance) == 0 {
			return nil, fmt.Errorf("cannot find ecs instance info from id %s", options.InstanceOfCluster)
		}
		info := response.Body.Instances.Instance[0]
		vpcID = *info.VpcAttributes.VpcId
	} else {
		return nil, fmt.Errorf("VPCID or InstanceOfCluster must be provided to get VPC ID")
	}

	cm := &CloudManager{
		vpc: vpcClient,
		ecs: ecsClient,
		slb: slbClient,

		region: options.Region,
		vpcID:  vpcID,

		ecsInfoCache: map[string]*ECSInfo{},
	}

	cm.vpcCIDRs, err = cm.GetCIDRsFromVPC(vpcID)
	if err != nil {
		return nil, err
	}

	return cm, nil
}

func (m *CloudManager) VPC() string {
	return m.vpcID
}

func (m *CloudManager) GetENIInfoFromID(networkInterfaceID string) (*ENIInfo, error) {
	ids := []*string{&networkInterfaceID}

	request := &ecs.DescribeNetworkInterfacesRequest{}
	request.SetRegionId(m.region).SetNetworkInterfaceId(ids)

	response, err := m.ecs.DescribeNetworkInterfaces(request)
	if err != nil {
		return nil, err
	}

	if len(response.Body.NetworkInterfaceSets.NetworkInterfaceSet) == 0 {
		return nil, fmt.Errorf("eni '%s' no found", networkInterfaceID)
	}

	networkInterface := response.Body.NetworkInterfaceSets.NetworkInterfaceSet[0]
	info := &ENIInfo{
		NetworkInterfaceSet: networkInterface,
		SecurityGroups:      map[string]SecurityGroupRule{},
	}

	for _, sgID := range networkInterface.SecurityGroupIds.SecurityGroupId {
		sgInfo, err := m.GetSecurityGroupRule(*sgID)
		if err != nil {
			return nil, err
		}

		info.SecurityGroups[sgInfo.ID] = sgInfo
	}
	info.RouteTableEntries, err = m.GetRouteEntryFromVswitch(*networkInterface.VSwitchId)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func (m *CloudManager) GetVSwitchFromID(id string) (*vpc.DescribeVSwitchesResponseBodyVSwitchesVSwitch, error) {
	request := &vpc.DescribeVSwitchesRequest{}
	request.SetRegionId(m.region).SetVSwitchId(id)

	response, err := m.vpc.DescribeVSwitches(request)
	if err != nil {
		return nil, err
	}

	if len(response.Body.VSwitches.VSwitch) == 0 {
		return nil, fmt.Errorf("vswitch '%s' not found", id)
	}

	return response.Body.VSwitches.VSwitch[0], nil
}

func (m *CloudManager) GetVPCDefaultRouteTableID(id string) (string, error) {
	request := &vpc.DescribeVpcsRequest{}
	request.SetRegionId(m.region).SetVpcId(id)

	response, err := m.vpc.DescribeVpcs(request)
	if err != nil {
		return "", err
	}

	if len(response.Body.Vpcs.Vpc) == 0 {
		return "", fmt.Errorf("vpc '%s' not found", id)
	}

	vRouterID := response.Body.Vpcs.Vpc[0].VRouterId
	routeRequest := &vpc.DescribeRouteTablesRequest{}
	routeRequest.SetRegionId(m.region).SetVRouterId(*vRouterID)

	routeResponse, err := m.vpc.DescribeRouteTables(routeRequest)
	if err != nil {
		return "", err
	}

	var defaultTable *vpc.DescribeRouteTablesResponseBodyRouteTablesRouteTable
	for _, t := range routeResponse.Body.RouteTables.RouteTable {
		if *t.RouteTableType == "System" {
			defaultTable = t
			break
		}
	}

	if defaultTable == nil {
		return "", fmt.Errorf("default route table for vpc '%s' not found", id)
	}

	return *defaultTable.RouteTableId, nil
}

func (m *CloudManager) GetSecurityGroupRule(id string) (SecurityGroupRule, error) {
	sgRequest := &ecs.DescribeSecurityGroupsRequest{}
	sgRequest.SetRegionId(m.region).SetSecurityGroupId(id)

	sgResponse, err := m.ecs.DescribeSecurityGroups(sgRequest)
	if err != nil {
		return SecurityGroupRule{}, err
	}

	if len(sgResponse.Body.SecurityGroups.SecurityGroup) == 0 {
		return SecurityGroupRule{}, fmt.Errorf("security group '%s' not found", id)
	}
	sg := sgResponse.Body.SecurityGroups.SecurityGroup[0]

	securityGroup := SecurityGroupRule{
		ID:   id,
		Type: SecurityGroupType(*sg.SecurityGroupType),
		InRule: SecurityGroupRules{
			Allows: nil,
			Drops:  nil,
		},
		OutRule: SecurityGroupRules{
			Allows: nil,
			Drops:  nil,
		},
	}

	sgAttrRequest := &ecs.DescribeSecurityGroupAttributeRequest{}
	sgAttrRequest.SetRegionId(m.region).SetSecurityGroupId(id)

	sgAttrResponse, err := m.ecs.DescribeSecurityGroupAttribute(sgAttrRequest)
	if err != nil {
		return securityGroup, err
	}

	polices := sgAttrResponse.Body.Permissions.Permission

	for _, policy := range polices {
		// when direction is 'ingress', use InRule
		rule := &securityGroup.InRule
		if *policy.Direction == "egress" {
			rule = &securityGroup.OutRule
		}

		if *policy.Policy == "Accept" {
			rule.Allows = append(rule.Allows, policy)
		} else {
			// policy is 'Drop'
			rule.Drops = append(rule.Drops, policy)
		}
	}

	return securityGroup, nil
}

var cachedRouteTableEntries = sync.Map{}

func (m *CloudManager) GetRouteTableEntries(routeTableID string) ([]*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry, error) {
	if entries, ok := cachedRouteTableEntries.Load(routeTableID); ok {
		return entries.([]*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry), nil
	}
	request := &vpc.DescribeRouteEntryListRequest{}
	request.SetRegionId(m.region).SetRouteTableId(routeTableID).SetNextToken("").SetMaxResult(100)

	var entries []*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry
	for {
		response, err := m.vpc.DescribeRouteEntryList(request)
		if err != nil {
			return nil, err
		}

		entries = append(entries, response.Body.RouteEntrys.RouteEntry...)

		if *response.Body.NextToken == "" {
			break
		}

		request.SetNextToken(*response.Body.NextToken)
	}
	cachedRouteTableEntries.Store(routeTableID, entries)

	return entries, nil
}

func (m *CloudManager) GetECSInfo(id string) (*ECSInfo, error) {
	if info, ok := m.ecsInfoCache[id]; ok {
		return info, nil
	}

	request := &ecs.DescribeInstancesRequest{}
	request.SetRegionId(m.region).SetInstanceIds(fmt.Sprintf("[%q]", id))

	response, err := m.ecs.DescribeInstances(request)
	if err != nil {
		return nil, err
	}

	if len(response.Body.Instances.Instance) == 0 {
		return nil, fmt.Errorf("instance '%s' not found", id)
	}
	instance := response.Body.Instances.Instance[0]

	ecsInfo := &ECSInfo{
		ID:     *instance.InstanceId,
		Status: *instance.Status,
		Network: ECSNetwork{
			SecurityGroups: map[string]SecurityGroupRule{},
			VSwitchID:      *instance.VpcAttributes.VSwitchId,
			VpcID:          *instance.VpcAttributes.VpcId,
		},
	}

	for _, ip := range instance.VpcAttributes.PrivateIpAddress.IpAddress {
		ecsInfo.Network.IP = append(ecsInfo.Network.IP, *ip)
	}

	for _, sg := range instance.SecurityGroupIds.SecurityGroupId {
		sgInfo, err := m.GetSecurityGroupRule(*sg)
		if err != nil {
			return ecsInfo, err
		}

		ecsInfo.Network.SecurityGroups[sgInfo.ID] = sgInfo
	}

	ecsInfo.Network.RouteTableEntries, err = m.GetRouteEntryFromVswitch(*instance.VpcAttributes.VSwitchId)
	if err != nil {
		return nil, err
	}

	ecsInfo.Network.VpcDefaultRouteTableEntries, err = m.GetVPCDefaultRouteEntry(*instance.VpcAttributes.VpcId)
	if err != nil {
		return ecsInfo, err
	}

	// get eni
	for _, networkInterface := range instance.NetworkInterfaces.NetworkInterface {
		info, err := m.GetENIInfoFromID(*networkInterface.NetworkInterfaceId)
		if err != nil {
			return ecsInfo, err
		}

		ecsInfo.Network.NetworkInterfaces = append(ecsInfo.Network.NetworkInterfaces, info)
	}

	if instance.EipAddress != nil && instance.EipAddress.IpAddress != nil {
		ecsInfo.Network.EIPAddress = *instance.EipAddress.IpAddress
	}

	m.ecsInfoCache[id] = ecsInfo
	return ecsInfo, nil
}

func (m *CloudManager) GetRouteEntryFromVswitch(id string) ([]*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry, error) {
	vswitch, err := m.GetVSwitchFromID(id)
	if err != nil {
		return nil, err
	}

	return m.GetRouteTableEntries(*vswitch.RouteTable.RouteTableId)
}

func (m *CloudManager) GetVPCDefaultRouteEntry(vpcID string) ([]*vpc.DescribeRouteEntryListResponseBodyRouteEntrysRouteEntry, error) {
	routeTableID, err := m.GetVPCDefaultRouteTableID(vpcID)
	if err != nil {
		return nil, err
	}

	return m.GetRouteTableEntries(routeTableID)
}

func (m *CloudManager) GetSLBFromPublicIP(ip string) (*slb.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancer, error) {
	request := &slb.DescribeLoadBalancersRequest{}
	request.SetRegionId(m.region).SetAddress(ip).SetAddressType("internet")

	response, err := m.slb.DescribeLoadBalancers(request)
	if err != nil {
		return nil, err
	}

	if *response.Body.TotalCount == 0 {
		return nil, nil
	}

	return response.Body.LoadBalancers.LoadBalancer[0], nil
}

func (m *CloudManager) GetSLBFromPrivateIP(ip string) (*slb.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancer, error) {
	request := &slb.DescribeLoadBalancersRequest{}
	request.SetRegionId(m.region).SetAddress(ip).SetAddressType("intranet")

	response, err := m.slb.DescribeLoadBalancers(request)
	if err != nil {
		return nil, err
	}

	if *response.Body.TotalCount == 0 {
		return nil, nil
	}

	return response.Body.LoadBalancers.LoadBalancer[0], nil
}

func (m *CloudManager) GetSLBFromID(id string) (*slb.DescribeLoadBalancersResponseBodyLoadBalancersLoadBalancer, error) {
	request := &slb.DescribeLoadBalancersRequest{}
	request.SetRegionId(m.region).SetLoadBalancerId(id)

	response, err := m.slb.DescribeLoadBalancers(request)
	if err != nil {
		return nil, err
	}

	if *response.Body.TotalCount == 0 {
		return nil, nil
	}

	return response.Body.LoadBalancers.LoadBalancer[0], nil
}

func (m *CloudManager) GetSLBListener(id string, port int32, protocol string) (*Listener, error) {
	ret := &Listener{}

	if strings.EqualFold(protocol, "udp") {
		request := &slb.DescribeLoadBalancerUDPListenerAttributeRequest{}
		request.SetRegionId(m.region).SetListenerPort(port).SetLoadBalancerId(id)

		response, err := m.slb.DescribeLoadBalancerUDPListenerAttribute(request)
		if err != nil {
			if strings.Contains(err.Error(), "The specified resource does not exist") {
				return nil, nil
			}
			return nil, err
		}
		ret.UDP = response.Body
	} else {
		request := &slb.DescribeLoadBalancerTCPListenerAttributeRequest{}
		request.SetRegionId(m.region).SetListenerPort(port).SetLoadBalancerId(id)

		response, err := m.slb.DescribeLoadBalancerTCPListenerAttribute(request)
		if err != nil {
			if strings.Contains(err.Error(), "The specified resource does not exist") {
				return nil, nil
			}
			return nil, err
		}
		ret.TCP = response.Body
	}

	return ret, nil
}

func (m *CloudManager) GetSLBVserverGroup(id string) (*slb.DescribeVServerGroupAttributeResponseBody, error) {
	request := &slb.DescribeVServerGroupAttributeRequest{}
	request.SetRegionId(m.region).SetVServerGroupId(id)

	response, err := m.slb.DescribeVServerGroupAttribute(request)
	if err != nil {
		if strings.Contains(err.Error(), "The specified VServerGroupId does not exist") {
			return nil, nil
		}
		return nil, err
	}

	return response.Body, nil
}

func (m *CloudManager) GetSLBHealthStatus(id string, port int32, protocol string) ([]*slb.DescribeHealthStatusResponseBodyBackendServersBackendServer, error) {
	request := &slb.DescribeHealthStatusRequest{}
	request.SetRegionId(m.region).SetLoadBalancerId(id)
	if port != 0 {
		request.SetListenerPort(port)
	}

	if protocol != "" {
		request.SetListenerProtocol(protocol)
	}

	response, err := m.slb.DescribeHealthStatus(request)
	if err != nil {
		if strings.Contains(err.Error(), "ListenerNotFound") {
			return nil, nil
		}
		return nil, err
	}

	return response.Body.BackendServers.BackendServer, nil
}

func (m *CloudManager) GetENIInfoFromVPCAndPrivateIP(vpcID, privateIP string) (*ecs.DescribeNetworkInterfacesResponseBodyNetworkInterfaceSetsNetworkInterfaceSet, error) {
	request := &ecs.DescribeNetworkInterfacesRequest{}
	request.SetRegionId(m.region).SetVpcId(vpcID).SetPrivateIpAddress([]*string{&privateIP})

	response, err := m.ecs.DescribeNetworkInterfaces(request)
	if err != nil {
		return nil, err
	}

	if *response.Body.TotalCount == 0 {
		return nil, fmt.Errorf("cannot find eni with vpc %s and private ip %s", vpcID, privateIP)
	}
	return response.Body.NetworkInterfaceSets.NetworkInterfaceSet[0], nil
}

func (m *CloudManager) GetNatGatewayInfo(id string) (*vpc.DescribeNatGatewaysResponseBodyNatGatewaysNatGateway, error) {
	request := &vpc.DescribeNatGatewaysRequest{}
	request.SetRegionId(m.region).SetNatGatewayId(id)

	response, err := m.vpc.DescribeNatGateways(request)
	if err != nil {
		return nil, err
	}

	if *response.Body.TotalCount == 0 {
		return nil, fmt.Errorf("cannot find nat gateway %s", id)
	}

	return response.Body.NatGateways.NatGateway[0], nil
}

func (m *CloudManager) GetSNATEntriesBySegment(snatTableID string, segment string) ([]*vpc.DescribeSnatTableEntriesResponseBodySnatTableEntriesSnatTableEntry, error) {
	request := &vpc.DescribeSnatTableEntriesRequest{}
	request.SetRegionId(m.region).SetSnatTableId(snatTableID).SetSourceCIDR(segment)

	response, err := m.vpc.DescribeSnatTableEntries(request)
	if err != nil {
		return nil, err
	}

	return response.Body.SnatTableEntries.SnatTableEntry, nil
}

func (m *CloudManager) GetVSwitch(id string) (*vpc.DescribeVSwitchesResponseBodyVSwitchesVSwitch, error) {
	request := &vpc.DescribeVSwitchesRequest{}
	request.SetRegionId(m.region).SetVSwitchId(id)

	response, err := m.vpc.DescribeVSwitches(request)
	if err != nil {
		return nil, err
	}

	if *response.Body.TotalCount == 0 {
		return nil, fmt.Errorf("cannot find vswitch %s", id)
	}

	return response.Body.VSwitches.VSwitch[0], nil
}

func (m *CloudManager) VPCCIDRs() []*net.IPNet {
	return m.vpcCIDRs
}

func (m *CloudManager) GetCIDRsFromVPC(id string) ([]*net.IPNet, error) {
	request := &vpc.DescribeVpcsRequest{}
	request.SetRegionId(m.region).SetVpcId(id)

	response, err := m.vpc.DescribeVpcs(request)
	if err != nil {
		return nil, err
	}

	if *response.Body.TotalCount == 0 {
		return nil, fmt.Errorf("cannot find vpc %s", id)
	}

	v := *response.Body.Vpcs.Vpc[0]

	var ipNets []*net.IPNet
	_, n, err := net.ParseCIDR(*v.CidrBlock)
	if err != nil {
		return nil, err
	}

	ipNets = append(ipNets, n)

	if v.UserCidrs == nil {
		return ipNets, nil
	}

	for _, cidr := range v.UserCidrs.UserCidr {
		_, n, err := net.ParseCIDR(*cidr)
		if err != nil {
			return nil, err
		}
		ipNets = append(ipNets, n)
	}
	return ipNets, nil
}

func (m *CloudManager) GetACLFromID(id string) (*slb.DescribeAccessControlListAttributeResponseBody, error) {
	request := &slb.DescribeAccessControlListAttributeRequest{}
	request.SetRegionId(m.region).SetAclId(id)

	response, err := m.slb.DescribeAccessControlListAttribute(request)
	if err != nil {
		if strings.Contains(err.Error(), "Acl does not exist") {
			return nil, nil
		}
		return nil, err
	}

	return response.Body, nil
}
