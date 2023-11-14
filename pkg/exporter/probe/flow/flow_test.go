package flow

import (
	"syscall"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestDefaultRouterDev(t *testing.T) {
	filter := &netlink.Route{
		Dst: nil,
	}
	routers, err := netlink.RouteListFiltered(syscall.AF_INET, filter, netlink.RT_FILTER_DST)
	if err != nil {
		t.Logf("err: %v", err)
		return
	}

	for _, r := range routers {
		t.Logf("t: %d, link: %d, r: %s", r.Type, r.LinkIndex, r.Dst.String())
	}
}

func TestLinkWatch(t *testing.T) {
	changes := make(chan netlink.LinkUpdate)
	done := make(chan struct{})
	if err := netlink.LinkSubscribe(changes, done); err != nil {
		t.Logf("failed watch, err: %v", err)
	}

	go func() {
		for c := range changes {
			t.Logf("mstype: %d, change %d, index %d,name: %s. newlink: %d",
				c.Header.Type, c.Change, c.Index, c.Link.Attrs().Name, syscall.RTM_NEWLINK)
		}
	}()
	<-done
}
