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
	CONNTRACK_NEW         = "CONNTRACK_NEW"
	CONNTRACK_UPDATE      = "CONNTRACK_UPDATE"
	CONNTRACK_DESTROY     = "CONNTRACK_DESTROY"
	CONNTRACK_EXP_NEW     = "CONNTRACK_EXP_NEW"
	CONNTRACK_EXP_DESTROY = "CONNTRACK_EXP_DESTROY"
	CONNTRACK_UNKNOW      = "CONNTRACK_UNKNOW"
)

var (
	MODULE_NAME = "insp_conntrack"

	events = []string{"CONNTRACK_NEW", "CONNTRACK_UPDATE", "CONNTRACK_DESTROY", "CONNTRACK_EXP_NEW", "CONNTRACK_EXP_DESTROY", "CONNTRACK_UNKNOW"}
	probe  = &NlConntrackProbe{mtx: sync.Mutex{}, conns: make(map[int]chan struct{})}
)

type NlConntrackProbe struct {
	enable   bool
	sub      chan<- proto.RawEvent
	mtx      sync.Mutex
	conns    map[int]chan struct{}
	initConn *conntrack.Conn
}

func GetProbe() *NlConntrackProbe {
	return probe
}

func (p *NlConntrackProbe) Name() string {
	return MODULE_NAME
}

func (p *NlConntrackProbe) Ready() bool {
	if _, err := p.getConn(); err != nil {
		return false
	}
	return true
}

func (p *NlConntrackProbe) Close() error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.enable = false

	return nil
}

func (p *NlConntrackProbe) GetEventNames() []string {
	return events
}

func (p *NlConntrackProbe) Start(ctx context.Context) {

	p.mtx.Lock()
	p.enable = true
	p.mtx.Unlock()

	ticker := time.NewTicker(10 * time.Second)
	defer p.release()
	for range ticker.C {
		if !p.enable {
			return
		}
		slog.Ctx(ctx).Info("start update netns list", "module", MODULE_NAME)
		ets := nettop.GetAllEntity()
		for _, et := range ets {
			if et == nil {
				slog.Ctx(ctx).Info("skip empty entity", "module", MODULE_NAME)
				continue
			}
			nsfd, err := et.GetNetNsFd()
			if err != nil {
				slog.Ctx(ctx).Info("skip netns fd", "err", err, "module", MODULE_NAME)
				continue
			}
			if nsfd == 0 {
				slog.Ctx(ctx).Info("skip empty netns fd", "module", MODULE_NAME)
				continue
			}
			if _, ok := p.conns[et.GetNetns()]; !ok {
				ctrch := make(chan struct{})
				go func() {
					err := p.startCtListen(ctx, ctrch, nsfd, et.GetNetns())
					if err != nil {
						slog.Ctx(ctx).Warn("start worker", "err", err, "netns", et.GetNetns(), "nsfd", nsfd, "module", MODULE_NAME)
						return
					}
				}()
				p.conns[et.GetNetns()] = ctrch
				slog.Ctx(ctx).Info("start worker finished", "netns", et.GetNetns(), "nsfd", nsfd, "module", MODULE_NAME)
			}
		}
	}

}

func (p *NlConntrackProbe) release() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	for _, ctrch := range p.conns {
		close(ctrch)
	}
	p.conns = make(map[int]chan struct{})
}

func (p *NlConntrackProbe) startCtListen(ctx context.Context, ctrch <-chan struct{}, nsfd int, nsinum int) error {
	c, err := conntrack.Dial(&netlink.Config{
		NetNS: nsfd,
	})

	if err != nil {
		slog.Ctx(ctx).Info("start conntrack dial", "err", err, "module", MODULE_NAME)
		return err
	}

	slog.Ctx(ctx).Info("start conntrack listen", "netns", nsfd, "module", MODULE_NAME)
	evCh := make(chan conntrack.Event, 1024)
	errCh, err := c.Listen(evCh, 4, append(netfilter.GroupsCT, netfilter.GroupsCTExp...))
	if err != nil {
		slog.Ctx(ctx).Info("start conntrack listen", "err", err, "module", MODULE_NAME)
		return err
	}

	for {
		select {
		case <-ctrch:
			slog.Ctx(ctx).Info("conntrack event listen stop", "module", MODULE_NAME)
			return nil
		case err = <-errCh:
			slog.Ctx(ctx).Info("conntrack event listen stop", "err", err, "module", MODULE_NAME)
			return err
		case event := <-evCh:
			if p.sub != nil {
				p.sub <- vanishEvent(event, nsinum)
				slog.Ctx(ctx).Info("conntrack event listen", "event", event.String(), "module", MODULE_NAME)
			}
		}
	}
}

func getEventCh(ctx context.Context, nsinum int) (evCh chan conntrack.Event, errCh chan error, err error) {
	c, err := conntrack.Dial(&netlink.Config{
		NetNS: nsinum,
	})

	if err != nil {
		slog.Ctx(ctx).Info("start conntrack dial", "err", err, "module", MODULE_NAME)
		return
	}

	slog.Ctx(ctx).Info("start conntrack listen", "netns", nsinum, "module", MODULE_NAME)
	evCh = make(chan conntrack.Event, 1024)
	errCh, err = c.Listen(evCh, 4, append(netfilter.GroupsCT, netfilter.GroupsCTExp...))
	if err != nil {
		slog.Ctx(ctx).Info("start conntrack listen", "err", err, "module", MODULE_NAME)
		return
	}

	return
}

// Register register sub chan to get perf events
func (p *NlConntrackProbe) Register(receiver chan<- proto.RawEvent) error {
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
		raw.EventType = CONNTRACK_NEW
	case conntrack.EventUpdate:
		raw.EventType = CONNTRACK_UPDATE
	case conntrack.EventDestroy:
		raw.EventType = CONNTRACK_DESTROY
	case conntrack.EventExpNew:
		raw.EventType = CONNTRACK_EXP_NEW
	case conntrack.EventExpDestroy:
		raw.EventType = CONNTRACK_EXP_DESTROY
	default:
		raw.EventType = CONNTRACK_UNKNOW
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
