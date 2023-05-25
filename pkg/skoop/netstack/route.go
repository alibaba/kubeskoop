package netstack

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

type Scope uint8

const (
	FamilyAll = unix.AF_UNSPEC
	FamilyV4  = unix.AF_INET
	FamilyV6  = unix.AF_INET6

	RtTableLocal   = 0xff
	RtTableDefault = 0xfd
	RtTableMain    = 0xfe

	ScopeUniverse Scope = 0x0
	ScopeSite     Scope = 0xc8
	ScopeLink     Scope = 0xfd
	ScopeHost     Scope = 0xfe
	ScopeNowhere  Scope = 0xff

	RTProtBIRD     = 0xc
	RTProtBoot     = 0x3
	RTProtKernel   = 0x2
	RTProtOSPF     = 0xbc
	RTProtRA       = 0x9
	RTProtRedirect = 0x1
	RTProtRIP      = 0xbd
	RTProtStatic   = 0x4
)

var ErrNoRouteToHost = errors.New("no route to host")

type Route struct {
	Family   int        `json:"f"`
	OifName  string     `json:"o"`
	IifName  string     `json:"i"`
	Scope    Scope      `json:"sc"`
	Dst      *net.IPNet `json:"d"`
	Src      net.IP     `json:"s"`
	Gw       net.IP     `json:"g"`
	Protocol int        `json:"p"`
	Priority int        `json:"pr"`
	Table    int        `json:"tb"`
	Type     int        `json:"t"`
	Tos      int        `json:"tos"`
	Flags    int        `json:"fl"`
}

func (r Route) String() string {
	var formattedString []string
	if r.Dst != nil {
		formattedString = append(formattedString, r.Dst.String())
	} else {
		formattedString = append(formattedString, "default")
	}

	if r.OifName != "" {
		formattedString = append(formattedString, fmt.Sprintf("dev %s", r.OifName))
	}

	if r.Gw != nil && !r.Gw.IsUnspecified() {
		formattedString = append(formattedString, fmt.Sprintf("via %s", r.Gw))
	}

	if scope := RouteScopeToString(r.Scope); scope != "" {
		formattedString = append(formattedString, fmt.Sprintf("scope %s", scope))
	}

	if routeType := RouteTypeToString(r.Type); routeType != "" {
		formattedString = append(formattedString, fmt.Sprintf("type %s", routeType))
	}

	// todo: prefsrc
	return strings.Join(formattedString, " ")
}

func RouteTypeToString(routeType int) string {
	switch routeType {
	case RtnLocal:
		return "local"
	case RtnUnicast:
		return "unicast"
	case RtnBroadcast:
		return "broadcast"
	case RtnAnycast:
		return "anycast"
	case RtnUnreachable:
		return "unreachable"
	case RtnBlackhole:
		return "blackhole"
	case RtnProhibit:
		return "prohibit"
	}

	return ""
}

func RouteScopeToString(scope Scope) string {
	switch scope {
	case ScopeLink:
		return "link"
	case ScopeHost:
		return "host"
	case ScopeUniverse:
		return "universe"
	}

	return ""
}

func RouteProtocolToString(protocol int) string {
	switch protocol {
	case RTProtRedirect:
		return "redirect"
	case RTProtKernel:
		return "kernel"
	case RTProtBoot:
		return "boot"
	case RTProtStatic:
		return "static"
	case RTProtRA:
		return "ra"
	case RTProtOSPF:
		return "ospf"
	case RTProtRIP:
		return "rip"
	case RTProtBIRD:
		return "bird"
	}

	return ""
}

type Rule struct {
	Priority int        `json:"p"`
	Family   int        `json:"f"`
	Table    int        `json:"tb"`
	Mark     int        `json:"m"`
	Mask     int        `json:"ma"`
	Tos      uint       `json:"tos"`
	TunID    uint       `json:"ti"`
	Goto     int        `json:"gt"`
	Src      *net.IPNet `json:"s"`
	Dst      *net.IPNet `json:"d"`
	Flow     int        `json:"fl"`
	IifName  string     `json:"i"`
	OifName  string     `json:"o"`
}

type Router interface {
	RouteSrc(packet *model.Packet, iif, oif string) (string, *Route, error)
	Route(packet *model.Packet, iif, oif string) (*Route, error)
	TableRoute(tableID int, packet *model.Packet) (*Route, error)
	DefaultRoute(table int) *Route
}

type SimulateRouter struct {
	rules      []Rule
	routes     map[int][]Route
	interfaces []Interface
}

func sortRule(rules []Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Priority < rules[j].Priority
	})
}

func sortRoute(routes []Route) {
	sort.SliceStable(routes, func(i, j int) bool {
		leftOnes, _ := routes[i].Dst.Mask.Size()
		rightOnes, _ := routes[j].Dst.Mask.Size()
		if leftOnes == rightOnes {
			return routes[i].Priority < routes[j].Priority
		}
		return leftOnes > rightOnes
	})
}

func matchRule(packet *model.Packet, rule Rule, iif, oif string) bool {
	if rule.Src != nil && !rule.Src.Contains(packet.Src) {
		return false
	}
	if rule.Dst != nil && !rule.Dst.Contains(packet.Dst) {
		return false
	}
	if rule.Mark > 0 && (int(packet.Mark)^rule.Mark)&rule.Mask != 0 {
		return false
	}
	if rule.IifName != "" && rule.IifName != iif {
		return false
	}

	if rule.OifName != "" && rule.OifName != oif {
		return false
	}
	return true
}

func NewSimulateRouter(rules []Rule, routes []Route, interfaces []Interface) *SimulateRouter {
	router := &SimulateRouter{
		rules:      rules,
		routes:     make(map[int][]Route),
		interfaces: interfaces,
	}
	sortRule(router.rules)

	routeTableMap := make(map[int][]Route)
	for _, route := range routes {
		routeTableMap[route.Table] = append(routeTableMap[route.Table], route)
	}

	for table, routes := range routeTableMap {
		sortRoute(routes)
		router.routes[table] = routes
	}

	return router
}

func (r *SimulateRouter) lookupRoute(table int, packet *model.Packet) *Route {
	routes, ok := r.routes[table]
	if !ok {
		return nil
	}
	for _, route := range routes {
		//不用判断nil，构造route的时候把Dst设置好
		if route.Dst.Contains(packet.Dst) {
			return &route
		}
	}
	return nil
}

func (r *SimulateRouter) Route(packet *model.Packet, iif, oif string) (*Route, error) {
	for _, rule := range r.rules {
		if matchRule(packet, rule, iif, oif) {
			route := r.lookupRoute(rule.Table, packet)
			if route != nil {
				return route, nil
			}
		}
	}
	return nil, ErrNoRouteToHost
}

func (r *SimulateRouter) DefaultRoute(table int) *Route {
	if table == 0 {
		table = RtTableLocal
	}
	for _, rt := range r.routes[table] {
		if rt.Dst == nil || rt.Dst.String() == "0.0.0.0/0" {
			return &rt
		}
	}
	return nil
}

func (r *SimulateRouter) TableRoute(table int, packet *model.Packet) (*Route, error) {
	route := r.lookupRoute(table, packet)
	if route == nil {
		return nil, ErrNoRouteToHost
	}
	return route, nil
}

func (r *SimulateRouter) RouteSrc(packet *model.Packet, iif, oif string) (string, *Route, error) {
	route, err := r.Route(packet, iif, oif)
	if err != nil {
		return "", nil, err
	}
	interfaceAddr := func(rt *Route) string {
		for _, oif := range r.interfaces {
			if oif.Name == rt.OifName {
				for _, addr := range oif.Addrs {
					if addr.IP.To4() != nil {
						return addr.IP.String()
					}
				}
			}
		}
		return ""
	}
	dstRouteSrc := interfaceAddr(route)
	if dstRouteSrc == "" {
		// use default route interface addr
		route = r.DefaultRoute(RtTableMain)
		dstRouteSrc = interfaceAddr(route)
	}
	if dstRouteSrc == "" {
		return "", nil, fmt.Errorf("cannot found valid ip for dst %v route", packet.Dst)
	}
	return dstRouteSrc, route, nil
}
