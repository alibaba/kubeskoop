package netstack

import (
	"fmt"
	"net"
	"testing"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	"github.com/stretchr/testify/assert"
)

func TestSortRouter(t *testing.T) {
	_, cidr10, _ := net.ParseCIDR("10.0.0.0/22")
	_, mask16, _ := net.ParseCIDR("192.168.0.0/16")
	_, mask24, _ := net.ParseCIDR("192.168.0.0/24")

	routes := []Route{
		{Dst: cidr10},
		{Dst: mask16},
		{Dst: mask24, Priority: 100},
		{Dst: mask24, Priority: 600},
	}
	sortRoute(routes)
	assert.Equal(t, Route{Dst: mask24, Priority: 100}, routes[0])
	assert.Equal(t, Route{Dst: mask24, Priority: 600}, routes[1])
	assert.Equal(t, Route{Dst: cidr10}, routes[2])
	assert.Equal(t, Route{Dst: mask16}, routes[3])

	rules := []Rule{
		{Priority: 512},
		{Priority: 1024},
		{Priority: 512},
		{Priority: 1024},
	}
	sortRule(rules)
	assert.Equal(t, Rule{Priority: 512}, rules[1])
}

func TestMatchRule(t *testing.T) {
	match := matchRule(&model.Packet{
		Mark: 0x1,
	}, Rule{
		Mark: 0xf1,
		Mask: 0x3,
	}, "", "")
	assert.Equal(t, match, true)
}

func TestRoutePacket(t *testing.T) {
	route16 := Route{
		Dst:   &net.IPNet{IP: net.IPv4(10, 0, 0, 1), Mask: net.CIDRMask(16, 32)},
		Table: 100,
	}
	Route24 := Route{
		Dst:   &net.IPNet{IP: net.IPv4(10, 0, 0, 1), Mask: net.CIDRMask(24, 32)},
		Table: 100,
	}
	RouteDefault := Route{
		Dst:   &net.IPNet{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(0, 32)},
		Table: 254,
	}
	routes := []Route{route16, Route24, RouteDefault}
	router := NewSimulateRouter([]Rule{
		{
			Src:   &net.IPNet{IP: net.IPv4(192, 168, 0, 1), Mask: net.CIDRMask(32, 32)},
			Table: 100,
		},
		//default
		{
			Table: 254,
		},
	}, routes, []Interface{})

	for _, r := range []struct {
		Packet model.Packet
		Route  Route
		err    error
	}{
		{model.Packet{Src: net.IPv4(192, 168, 0, 1), Dst: net.IPv4(10, 0, 0, 1)}, Route24, nil},
		{model.Packet{Src: net.IPv4(192, 168, 0, 1), Dst: net.IPv4(1, 0, 0, 1)}, Route{}, fmt.Errorf("")},
		{model.Packet{Src: net.IPv4(192, 168, 0, 2), Dst: net.IPv4(1, 0, 0, 1)}, RouteDefault, nil},
	} {
		actualRoute, err := router.Route(&r.Packet, "", "")
		if r.err == nil {
			assert.Equal(t, r.Route, actualRoute)
		} else {
			assert.NotEqual(t, err, nil)
		}
	}
}
