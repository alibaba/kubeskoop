package assertions

import (
	"fmt"
	"net"
	"reflect"
	"strings"

	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	netstack2 "github.com/alibaba/kubeskoop/pkg/skoop/netstack"

	"github.com/samber/lo"
	"golang.org/x/exp/slices"
)

type NetstackAssertion struct {
	Assertion
	netns *netstack2.NetNS
}

func NewNetstackAssertion(assertion Assertion, netns *netstack2.NetNS) *NetstackAssertion {
	return &NetstackAssertion{Assertion: assertion, netns: netns}
}

func (na *NetstackAssertion) AssertSysctls(expectSysctls map[string]string, suspicionLevel model2.SuspicionLevel) {
	for s, expect := range expectSysctls {
		actual, ok := na.netns.NetNSInfo.SysctlInfo[s]
		if !ok {
			na.AddSuspicion(suspicionLevel, fmt.Sprintf("expect sysctl %s not exist in actual netns", s))
			continue
		}

		AssertTrue(na, actual == expect, suspicionLevel,
			fmt.Sprintf("expect sysctl %s in actual netns: %s not equal to expect: %s", s, actual, expect))
	}
}

func (na *NetstackAssertion) AssertIPForwardedEnabled() {
	na.AssertSysctls(map[string]string{"net.ipv4.ip_forward": "1"}, model2.SuspicionLevelFatal)
}

func (na *NetstackAssertion) AssertRpFilterDisabled(dev string) {
	if dev == "" {
		dev = "all"
	}

	na.AssertSysctls(map[string]string{
		fmt.Sprintf("net.ipv4.conf.%s.rp_filter", dev): "0",
	}, model2.SuspicionLevelFatal)
}

func (na *NetstackAssertion) AssertDefaultRule() {
	for _, r := range na.netns.NetNSInfo.RuleInfo {
		// main & local table
		if r.Table == netstack2.RtTableMain || r.Table == netstack2.RtTableLocal {
			if r.Src != nil || r.IifName != "" || r.OifName != "" || r.Dst != nil {
				na.AddSuspicion(model2.SuspicionLevelCritical,
					"default policy to table main is wrong with non zero fields")
				return
			}
			return
		}
	}
	na.AddSuspicion(model2.SuspicionLevelCritical,
		"default policy to table main is deleted")
}

func (na *NetstackAssertion) AssertNetDevice(s string, expect netstack2.Interface) {
	for _, ni := range na.netns.Interfaces {
		if ni.Name == s {
			niType := reflect.TypeOf(expect)
			expectv := reflect.ValueOf(expect)
			actualv := reflect.ValueOf(ni)
			for i := 0; i < niType.NumField(); i++ {
				AssertTrue(na, expectv.Field(i).IsZero() || (expectv.Field(i).Interface() == actualv.Field(i).Interface()),
					model2.SuspicionLevelFatal,
					fmt.Sprintf("netdevice %q field: %v is not expect: actual(%v) != expect(%v)", s, niType.Field(i).Name, actualv.Field(i), expectv.Field(i)),
				)
			}
			return
		}
	}

	na.AddSuspicion(model2.SuspicionLevelCritical,
		fmt.Sprintf("cannot found interface: %s to assert", s),
	)
}

func (na *NetstackAssertion) AssertNoPolicyRoute() {
	defaultRoutes := []int{netstack2.RtTableMain, netstack2.RtTableLocal, netstack2.RtTableDefault}
	var policyRoutes []int
	for _, rule := range na.netns.NetNSInfo.RuleInfo {
		if !slices.Contains(defaultRoutes, rule.Table) {
			policyRoutes = append(policyRoutes, rule.Table)
		}
	}
	AssertTrue(na, len(policyRoutes) == 0, model2.SuspicionLevelWarning,
		fmt.Sprintf("policy route enabled, tables: %+v", policyRoutes))
}

func (na *NetstackAssertion) AssertListen(localIP net.IP, localPort uint16, protocol model2.Protocol) {
	socks := lo.Filter(na.netns.NetNSInfo.ConnStats, func(stat netstack2.ConnStat, index int) bool {
		if stat.State == netstack2.SockStatListen && strings.EqualFold(string(stat.Protocol), string(protocol)) && localPort == stat.LocalPort {
			if localIP.String() == stat.LocalIP {
				return true
			}
			if slices.Contains([]string{"0.0.0.0", "::"}, stat.LocalIP) {
				return true
			}
		}
		return false
	})

	AssertTrue(na, len(socks) != 0, model2.SuspicionLevelFatal,
		fmt.Sprintf("no process listening on 0.0.0.0:%v or %v:%v", localPort, localIP, localPort))
}

func (na *NetstackAssertion) AssertHostBridge(name string) {
	bridge, ok := lo.Find(na.netns.Interfaces, func(iface netstack2.Interface) bool { return iface.Name == name })
	if !ok {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("bridge %s is not existed", name))
		return
	}

	AssertTrue(na, bridge.State == netstack2.LinkUP, model2.SuspicionLevelFatal, fmt.Sprintf("bridge %s state is down", name))
}

func (na *NetstackAssertion) AssertVEthPeerBridge(peerInterfaceName string, peerNS *netstack2.NetNSInfo, expectedBridgeName string) {
	peer, ok := lo.Find(peerNS.Interfaces, func(iface netstack2.Interface) bool { return iface.Name == peerInterfaceName })
	if !ok {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("cannot find eth0 in peer netns %s", peerNS.NetnsID))
		return
	}

	dev, ok := lo.Find(na.netns.Interfaces, func(iface netstack2.Interface) bool { return iface.Index == peer.PeerIndex })
	if !ok {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("veth index %d is not existed", peer.PeerIndex),
		)
		return
	}

	if dev.Driver != netstack2.LinkDriverVeth {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("%s is not a veth interface", dev.Name))
		return
	}

	AssertTrue(na, dev.State == netstack2.LinkUP, model2.SuspicionLevelWarning,
		fmt.Sprintf("state of veth peer %s is DOWN", dev.Name))

	if dev.MasterIndex == 0 {
		na.AddSuspicion(model2.SuspicionLevelFatal, fmt.Sprintf("%s has no master", dev.Name))
		return
	}

	bridge, ok := lo.Find(na.netns.Interfaces, func(iface netstack2.Interface) bool { return iface.Index == dev.MasterIndex })
	if !ok {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("cannot find master interface %d in peer ns %s", dev.MasterIndex, na.netns.NetNSInfo.NetnsID))
		return
	}

	if expectedBridgeName != "" {
		AssertTrue(na, bridge.Name == expectedBridgeName, model2.SuspicionLevelFatal,
			fmt.Sprintf("bridge of %s is %s, not %s", dev.Name, bridge.Name, expectedBridgeName))
	}

	// todo: br_port_state
}

func (na *NetstackAssertion) AssertVEthOnBridge(index int, expectedBridgeName string) {
	dev, ok := lo.Find(na.netns.Interfaces, func(iface netstack2.Interface) bool { return iface.Index == index })
	if !ok {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("veth peer index %d is not existed", index),
		)
		return
	}

	if dev.Driver != netstack2.LinkDriverVeth {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("%s is not a veth interface", dev.Name))
		return
	}

	AssertTrue(na, dev.State == netstack2.LinkUP, model2.SuspicionLevelWarning,
		fmt.Sprintf("state of veth peer %s is DOWN", dev.Name))

	if dev.MasterIndex == 0 {
		na.AddSuspicion(model2.SuspicionLevelFatal, fmt.Sprintf("%s has no master", dev.Name))
		return
	}

	bridge, ok := lo.Find(na.netns.Interfaces, func(iface netstack2.Interface) bool { return iface.Index == dev.MasterIndex })
	if !ok {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("cannot find master interface %d, for dev %s", dev.MasterIndex, dev.Name))
		return
	}

	if expectedBridgeName != "" {
		AssertTrue(na, bridge.Name == expectedBridgeName, model2.SuspicionLevelFatal,
			fmt.Sprintf("bridge of %s is %s, not %s", dev.Name, bridge.Name, expectedBridgeName))
	}

	// todo: br_port_state
}

func (na *NetstackAssertion) AssertIPVSServiceExists(service, servicePort, protocol string) {
	key := fmt.Sprintf("%s:%s:%s", protocol, service, servicePort)
	AssertTrue(na, slices.Contains(na.netns.NetNSInfo.IPVSInfo, key), model2.SuspicionLevelWarning,
		fmt.Sprintf("ipvs has no service %s", key))
}

type RouteAssertion struct {
	Dev      *string
	Scope    *netstack2.Scope
	Type     *int
	Dst      *net.IPNet
	Src      *net.IP
	Gw       *net.IP
	Protocol *int
}

func (a RouteAssertion) String() string {
	var formattedString []string
	if a.Dev != nil {
		formattedString = append(formattedString, fmt.Sprintf("dev: %s", *a.Dev))
	}
	if a.Scope != nil {
		formattedString = append(formattedString, fmt.Sprintf("scope: %s", netstack2.RouteScopeToString(*a.Scope)))
	}
	if a.Type != nil {
		formattedString = append(formattedString, fmt.Sprintf("type: %s", netstack2.RouteTypeToString(*a.Type)))
	}
	if a.Src != nil {
		formattedString = append(formattedString, fmt.Sprintf("src: %s", *a.Src))
	}
	if a.Dst != nil {
		formattedString = append(formattedString, fmt.Sprintf("dst: %s", *a.Dst))
	}
	if a.Gw != nil {
		formattedString = append(formattedString, fmt.Sprintf("gateway: %s", *a.Gw))
	}
	if a.Protocol != nil {
		formattedString = append(formattedString, fmt.Sprintf("protocol: %s", netstack2.RouteProtocolToString(*a.Protocol)))
	}
	return strings.Join(formattedString, " ")
}

func (na *NetstackAssertion) AssertRoute(expected RouteAssertion, packet model2.Packet, iif, oif string) error {
	router := na.netns.Router
	route, err := router.Route(&packet, iif, oif)
	if err == netstack2.ErrNoRouteToHost {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("no route to host %s", packet.Dst))
		return nil
	}

	if err != nil {
		return err
	}

	if slices.Contains([]int{netstack2.RtnUnreachable, netstack2.RtnBlackhole, netstack2.RtnProhibit, netstack2.RtnThrow}, route.Type) {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("route with type %d which indicates %s is unreachable", route.Type, packet.Dst))
		return nil
	}

	if route.Type == netstack2.RtnLocal {
		return nil
	}

	if expected.Dev != nil && *expected.Dev != route.OifName {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("invalid route %q for packet {src=%s, dst=%s}, expected: %q",
				route, packet.Src, packet.Dst, expected))
		return nil
	}
	if expected.Scope != nil && *expected.Scope != route.Scope {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("invalid route %q for packet {src=%s, dst=%s}, expected: %q",
				route, packet.Src, packet.Dst, expected))
		return nil
	}
	if expected.Src != nil && !route.Src.Equal(*expected.Src) {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("invalid route %q for packet {src=%s, dst=%s}, expected: %q",
				route, packet.Src, packet.Dst, expected))
		return nil
	}
	if expected.Dst != nil && (*route.Dst).String() != (*expected.Dst).String() {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("invalid route %q for packet {src=%s, dst=%s}, expected: %q",
				route, packet.Src, packet.Dst, expected))
		return nil
	}
	if expected.Gw != nil && !route.Gw.Equal(*expected.Gw) {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("invalid route %q for packet {src=%s, dst=%s}, expected: %q",
				route, packet.Src, packet.Dst, expected))
		return nil
	}
	if expected.Type != nil && *expected.Type != route.Type {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("invalid route %q for packet {src=%s, dst=%s}, expected: %q",
				route, packet.Src, packet.Dst, expected))
		return nil
	}
	if expected.Protocol != nil && *expected.Protocol != route.Protocol {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("invalid route %q for packet {src=%s, dst=%s}, expected: %q",
				route, packet.Src, packet.Dst, expected))
		return nil
	}

	return nil
}

func (na *NetstackAssertion) AssertVxlanVtep(vtep, dstHost net.IP, vxlanInterface string) error {
	iface, ok := lo.Find(na.netns.Interfaces, func(i netstack2.Interface) bool { return i.Name == vxlanInterface })
	if !ok {
		na.AddSuspicion(model2.SuspicionLevelFatal, fmt.Sprintf("invalid vxlan interface %s", vxlanInterface))
		return nil
	}

	neighResult, err := na.netns.Neighbour.ProbeNeigh(vtep, iface.Index)
	if err != nil {
		return err
	}

	if neighResult == nil {
		na.AddSuspicion(model2.SuspicionLevelCritical,
			fmt.Sprintf("no neigh for next node hop: %s on %s", vtep, vxlanInterface))
		return nil
	}

	if neighResult.DST == nil || neighResult.DST.IsUnspecified() {
		na.AddSuspicion(model2.SuspicionLevelCritical,
			fmt.Sprintf("no fdb table for %s", vxlanInterface))
		return nil
	}

	if !neighResult.DST.Equal(dstHost) {
		na.AddSuspicion(model2.SuspicionLevelCritical,
			fmt.Sprintf("fdb table for %q not equal to expect vtep %q", vtep, dstHost))
	}

	return nil
}

func (na *NetstackAssertion) AssertDefaultIPIPTunnel(ifName string) {
	dev, ok := lo.Find(na.netns.Interfaces, func(i netstack2.Interface) bool { return i.Name == ifName })
	if !ok {
		na.AddSuspicion(
			model2.SuspicionLevelFatal,
			fmt.Sprintf("interface %q does not exist", ifName))
		return
	}

	if dev.Driver != netstack2.LinkDriverIPIP {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("driver of interface %q is %q, not %q", ifName, dev.Driver, netstack2.LinkDriverIPIP))
		return
	}

	// fixme: need driver specific info for ipip
}

// netfilter assertion functions

// AssertNoIPTables assertion no iptables rules
func (na *NetstackAssertion) AssertNoIPTables() {
	if err := na.netns.IPTables.Empty(); err != nil {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("iptables: %s", err))
	}
}

func (na *NetstackAssertion) AssertDefaultAccept() {
	if err := na.netns.IPTables.DefaultAccept(); err != nil {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("iptables: %s", err))
	}
}

func (na *NetstackAssertion) checkNetfilterResult(verdict netstack2.Verdict, err error) bool {
	if err != nil {
		if err == netstack2.ErrIPTablesUnsupported {
			na.AddSuspicion(model2.SuspicionLevelWarning,
				"iptables contains unsupported rules, which is not expected")
			return false
		}

		if e, ok := err.(*netstack2.IPTablesRuleError); ok {
			na.AddSuspicion(model2.SuspicionLevelWarning,
				fmt.Sprintf("iptables contains unsupported rule: %v", e))
			return false
		}

		if e, ok := err.(*netstack2.IPTableDropError); ok {
			na.AddSuspicion(model2.SuspicionLevelWarning,
				fmt.Sprintf("packet drop by iptables, trace: %v", e.Trace))
			return false
		}
	}

	return true
}

func (na *NetstackAssertion) AssertNetfilterSend(pktIn model2.Packet, pktOut []model2.Packet, iif string) {
	verdict, _, err := na.netns.Netfilter.Hook(netstack2.NFHookOutput, pktIn, iif, "")
	if !na.checkNetfilterResult(verdict, err) {
		return
	}
	for _, pkt := range pktOut {
		verdict, _, err := na.netns.Netfilter.Hook(netstack2.NFHookPostRouting, pkt, iif, "")
		if !na.checkNetfilterResult(verdict, err) {
			continue
		}

		//should we check snat here?
	}
}

func (na *NetstackAssertion) AssertNetfilterForward(pktIn model2.Packet, pktOut []model2.Packet, iif string) {
	verdict, filteredPkt, err := na.netns.Netfilter.Hook(netstack2.NFHookPreRouting, pktIn, iif, "")
	if !na.checkNetfilterResult(verdict, err) {
		return
	}

	if len(pktOut) == 0 {
		pktOut = []model2.Packet{pktIn}
	}

	pktCopy := pktIn
	for _, pkt := range pktOut {
		pktCopy.Dst = pkt.Dst
		verdict, filteredPkt, err = na.netns.Netfilter.Hook(netstack2.NFHookForward, pktCopy, iif, "")
		na.checkNetfilterResult(verdict, err)
		verdict, filteredPkt, err = na.netns.Netfilter.Hook(netstack2.NFHookPostRouting, filteredPkt, iif, "")
		na.checkNetfilterResult(verdict, err)

		if !pktCopy.Src.Equal(filteredPkt.Src) {
			na.AddSuspicion(model2.SuspicionLevelFatal,
				fmt.Sprintf("pkt %v is SNATed to %v, which is not expected", pktIn, filteredPkt))
		}
	}
}

func (na *NetstackAssertion) AssertNetfilterServe(pktIn model2.Packet, iif string) {
	verdict, filteredPkt, err := na.netns.Netfilter.Hook(netstack2.NFHookPreRouting, pktIn, iif, "")
	if !na.checkNetfilterResult(verdict, err) {
		return
	}

	verdict, _, err = na.netns.Netfilter.Hook(netstack2.NFHookInput, filteredPkt, iif, "")
	na.checkNetfilterResult(verdict, err)
}

func (na *NetstackAssertion) AssertIPVSServerExists(service string, servicePort uint16, protocol model2.Protocol,
	backend string, backendPort uint16) {
	key := fmt.Sprintf("%s:%s:%d", protocol, service, servicePort)
	_, ok := lo.Find(na.netns.NetNSInfo.IPVSInfo, func(i string) bool { return i == key })
	if !ok {
		return
	}

	svc := na.netns.IPVS.GetService(protocol, service, servicePort)
	if svc == nil || svc.RS == nil {
		na.AddSuspicion(model2.SuspicionLevelFatal,
			fmt.Sprintf("ipvs has no service %s or service has no rs info", key))
	}

	found := lo.ContainsBy(svc.RS, func(rs netstack2.RealServer) bool {
		return rs.IP == backend && rs.Port == backendPort
	})

	AssertTrue(na, found, model2.SuspicionLevelWarning,
		fmt.Sprintf("ipvs service %s has no endpoint %s:%d", service, backend, backendPort))
}
