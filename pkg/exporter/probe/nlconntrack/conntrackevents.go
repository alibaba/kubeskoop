package nlconntrack

import (
	"context"
	"fmt"

	"net"
	"strconv"
	"sync"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/mdlayher/netlink"
	"github.com/ti-mo/conntrack"
	"github.com/ti-mo/netfilter"
	"golang.org/x/exp/slog"
)

const (
	ConntrackNew        = "ConntrackNew"
	ConntrackUpdate     = "ConntrackUpdate"
	ConntrackDestroy    = "ConntrackDestroy"
	ConntrackExpNew     = "ConntrackExpNew"
	ConntrackExpDestroy = "ConntrackExpDestroy"
	ConntrackUnknow     = "ConntrackUnknow"
)

var (
	ModuleName = "insp_conntrack"

	events = []string{"ConntrackNew", "ConntrackUpdate", "ConntrackDestroy", "ConntrackExpNew", "ConntrackExpDestroy", "ConntrackUnknow"}
	probe  = &Probe{mtx: sync.Mutex{}, conns: make(map[int]chan struct{})}
)

type Probe struct {
	enable   bool
	sub      chan<- proto.RawEvent
	mtx      sync.Mutex
	conns    map[int]chan struct{}
	initConn *conntrack.Conn
}

func GetProbe() *Probe {
	return probe
}

func (p *Probe) Name() string {
	return ModuleName
}

func (p *Probe) Ready() bool {
	if _, err := p.getConn(); err != nil {
		return false
	}
	return true
}

func (p *Probe) Close(_ proto.ProbeType) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.enable = false

	return nil
}

func (p *Probe) GetEventNames() []string {
	return events
}

func (p *Probe) Start(ctx context.Context, _ proto.ProbeType) {
	p.mtx.Lock()
	p.enable = true
	p.mtx.Unlock()

	ticker := time.NewTicker(10 * time.Second)
	defer p.release()
	for range ticker.C {
		if !p.enable {
			return
		}
		slog.Ctx(ctx).Info("start update netns list", "module", ModuleName)
		ets := nettop.GetAllEntity()
		for _, et := range ets {
			if et == nil {
				slog.Ctx(ctx).Info("skip empty entity", "module", ModuleName)
				continue
			}
			nsfd, err := et.GetNetNsFd()
			if err != nil {
				slog.Ctx(ctx).Info("skip netns fd", "err", err, "module", ModuleName)
				continue
			}
			if nsfd == 0 {
				slog.Ctx(ctx).Info("skip empty netns fd", "module", ModuleName)
				continue
			}
			if _, ok := p.conns[et.GetNetns()]; !ok {
				ctrch := make(chan struct{})
				go func() {
					err := p.startCtListen(ctx, ctrch, nsfd, et.GetNetns())
					if err != nil {
						slog.Ctx(ctx).Warn("start worker", "err", err, "netns", et.GetNetns(), "nsfd", nsfd, "module", ModuleName)
						return
					}
				}()
				p.conns[et.GetNetns()] = ctrch
				slog.Ctx(ctx).Info("start worker finished", "netns", et.GetNetns(), "nsfd", nsfd, "module", ModuleName)
			}
		}
	}

}

func (p *Probe) release() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, ctrch := range p.conns {
		close(ctrch)
	}
	p.conns = make(map[int]chan struct{})
}

func (p *Probe) startCtListen(ctx context.Context, ctrch <-chan struct{}, nsfd int, nsinum int) error {
	c, err := conntrack.Dial(&netlink.Config{
		NetNS: nsfd,
	})

	if err != nil {
		slog.Ctx(ctx).Info("start conntrack dial", "err", err, "module", ModuleName)
		return err
	}

	slog.Ctx(ctx).Info("start conntrack listen", "netns", nsfd, "module", ModuleName)
	evCh := make(chan conntrack.Event, 1024)
	errCh, err := c.Listen(evCh, 4, append(netfilter.GroupsCT, netfilter.GroupsCTExp...))
	if err != nil {
		slog.Ctx(ctx).Info("start conntrack listen", "err", err, "module", ModuleName)
		return err
	}

	for {
		select {
		case <-ctrch:
			slog.Ctx(ctx).Info("conntrack event listen stop", "module", ModuleName)
			return nil
		case err = <-errCh:
			slog.Ctx(ctx).Info("conntrack event listen stop", "err", err, "module", ModuleName)
			return err
		case event := <-evCh:
			if p.sub != nil {
				p.sub <- vanishEvent(event, nsinum)
				slog.Ctx(ctx).Info("conntrack event listen", "event", event.String(), "module", ModuleName)
			}
		}
	}
}

// func getEventCh(ctx context.Context, nsinum int) (evCh chan conntrack.Event, errCh chan error, err error) {
// 	c, err := conntrack.Dial(&netlink.Config{
// 		NetNS: nsinum,
// 	})

// 	if err != nil {
// 		slog.Ctx(ctx).Info("start conntrack dial", "err", err, "module", ModuleName)
// 		return
// 	}

// 	slog.Ctx(ctx).Info("start conntrack listen", "netns", nsinum, "module", ModuleName)
// 	evCh = make(chan conntrack.Event, 1024)
// 	errCh, err = c.Listen(evCh, 4, append(netfilter.GroupsCT, netfilter.GroupsCTExp...))
// 	if err != nil {
// 		slog.Ctx(ctx).Info("start conntrack listen", "err", err, "module", ModuleName)
// 		return
// 	}

// 	return
// }

// Register register sub chan to get perf events
func (p *Probe) Register(receiver chan<- proto.RawEvent) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sub = receiver

	return nil
}

func vanishEvent(evt conntrack.Event, nsinum int) proto.RawEvent {
	raw := proto.RawEvent{
		Netns: uint32(nsinum),
	}
	switch evt.Type {
	case conntrack.EventNew:
		raw.EventType = ConntrackNew
	case conntrack.EventUpdate:
		raw.EventType = ConntrackUpdate
	case conntrack.EventDestroy:
		raw.EventType = ConntrackDestroy
	case conntrack.EventExpNew:
		raw.EventType = ConntrackExpNew
	case conntrack.EventExpDestroy:
		raw.EventType = ConntrackExpDestroy
	default:
		raw.EventType = ConntrackUnknow
	}

	rawStr := fmt.Sprintf("Proto = %s Replied = %t ", bpfutil.GetProtoStr(evt.Flow.TupleOrig.Proto.Protocol), evt.Flow.Status.SeenReply())
	if evt.Flow.TupleOrig.Proto.Protocol == 6 && evt.Flow.ProtoInfo.TCP != nil {
		rawStr += fmt.Sprintf("State = %s ", bpfutil.GetTCPState(evt.Flow.ProtoInfo.TCP.State))
	}
	rawStr += fmt.Sprintf("Src = %s, Dst = %s", net.JoinHostPort(evt.Flow.TupleOrig.IP.SourceAddress.String(), strconv.Itoa(int(evt.Flow.TupleOrig.Proto.SourcePort))),
		net.JoinHostPort(evt.Flow.TupleOrig.IP.DestinationAddress.String(), strconv.Itoa(int(evt.Flow.TupleOrig.Proto.DestinationPort))))
	raw.EventBody = rawStr
	return raw
}
