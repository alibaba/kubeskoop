package nlconntrack

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
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
	probeName = "conntrack"
)

func eventProbeCreator(sink chan<- *probe.Event, _ map[string]interface{}) (probe.EventProbe, error) {
	p := &conntrackEventProbe{
		sink: sink,
	}
	return probe.NewEventProbe(probeName, p), nil
}

type conntrackEventProbe struct {
	sink  chan<- *probe.Event
	conns map[int]chan struct{}
	done  chan struct{}
}

func (p *conntrackEventProbe) Start(ctx context.Context) error {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		select {
		case <-ticker.C:
			slog.Ctx(ctx).Info("start update netns list", "module", probeName)
			ets := nettop.GetAllEntity()
			for _, et := range ets {
				if et == nil {
					slog.Ctx(ctx).Info("skip empty entity", "module", probeName)
					continue
				}
				nsfd, err := et.GetNetNsFd()
				if err != nil {
					slog.Ctx(ctx).Info("skip netns fd", "err", err, "module", probeName)
					continue
				}
				if nsfd == 0 {
					slog.Ctx(ctx).Info("skip empty netns fd", "module", probeName)
					continue
				}
				if _, ok := p.conns[et.GetNetns()]; !ok {
					ctrch := make(chan struct{})
					go func() {
						err := p.startCtListen(ctx, ctrch, nsfd, et.GetNetns())
						if err != nil {
							slog.Ctx(ctx).Warn("start worker", "err", err, "netns", et.GetNetns(), "nsfd", nsfd, "module", probeName)
							return
						}
					}()
					p.conns[et.GetNetns()] = ctrch
					slog.Ctx(ctx).Info("start worker finished", "netns", et.GetNetns(), "nsfd", nsfd, "module", probeName)
				}
			}
		case <-p.done:
			return
		}
	}()

	return nil
}

func (p *conntrackEventProbe) Stop(_ context.Context) error {
	close(p.done)
	for _, conn := range p.conns {
		close(conn)
	}
	return nil
}

func (p *conntrackEventProbe) startCtListen(ctx context.Context, ctrch <-chan struct{}, nsfd int, nsinum int) error {
	c, err := conntrack.Dial(&netlink.Config{
		NetNS: nsfd,
	})

	if err != nil {
		slog.Ctx(ctx).Info("start conntrack dial", "err", err, "module", probeName)
		return err
	}

	slog.Ctx(ctx).Info("start conntrack listen", "netns", nsfd, "module", probeName)
	evCh := make(chan conntrack.Event, 1024)
	errCh, err := c.Listen(evCh, 4, append(netfilter.GroupsCT, netfilter.GroupsCTExp...))
	if err != nil {
		slog.Ctx(ctx).Info("start conntrack listen", "err", err, "module", probeName)
		return err
	}

	for {
		select {
		case <-ctrch:
			slog.Ctx(ctx).Info("conntrack event listen stop", "module", probeName)
			return nil
		case err = <-errCh:
			slog.Ctx(ctx).Info("conntrack event listen stop", "err", err, "module", probeName)
			return err
		case event := <-evCh:
			p.sink <- vanishEvent(event, nsinum)
			slog.Ctx(ctx).Info("conntrack event listen", "event", event.String(), "module", probeName)
		}
	}
}

var eventTypeMapping = map[uint8]probe.EventType{
	uint8(conntrack.EventNew):        ConntrackNew,
	uint8(conntrack.EventUpdate):     ConntrackUpdate,
	uint8(conntrack.EventDestroy):    ConntrackDestroy,
	uint8(conntrack.EventExpNew):     ConntrackExpNew,
	uint8(conntrack.EventExpDestroy): ConntrackExpDestroy,
	uint8(conntrack.EventUnknown):    ConntrackUnknow,
}

func vanishEvent(evt conntrack.Event, nsinum int) *probe.Event {

	rawStr := fmt.Sprintf("Proto = %s Replied = %t ", bpfutil.GetProtoStr(evt.Flow.TupleOrig.Proto.Protocol), evt.Flow.Status.SeenReply())
	if evt.Flow.TupleOrig.Proto.Protocol == 6 && evt.Flow.ProtoInfo.TCP != nil {
		rawStr += fmt.Sprintf("State = %s ", bpfutil.GetTCPState(evt.Flow.ProtoInfo.TCP.State))
	}
	rawStr += fmt.Sprintf("Src = %s, Dst = %s", net.JoinHostPort(evt.Flow.TupleOrig.IP.SourceAddress.String(), strconv.Itoa(int(evt.Flow.TupleOrig.Proto.SourcePort))),
		net.JoinHostPort(evt.Flow.TupleOrig.IP.DestinationAddress.String(), strconv.Itoa(int(evt.Flow.TupleOrig.Proto.DestinationPort))))

	return &probe.Event{
		Timestamp: time.Now().UnixNano(),
		Type:      eventTypeMapping[uint8(evt.Type)],
		Labels:    probe.EventMetaByNetNS(nsinum),
		Message:   rawStr,
	}
}

func init() {
	probe.MustRegisterEventProbe(probeName, eventProbeCreator)
}
