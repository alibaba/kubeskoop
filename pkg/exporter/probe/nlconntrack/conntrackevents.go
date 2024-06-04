package nlconntrack

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netns"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/mdlayher/netlink"
	"github.com/ti-mo/conntrack"
	"github.com/ti-mo/netfilter"
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

func eventProbeCreator(sink chan<- *probe.Event) (probe.EventProbe, error) {
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
			log.Infof("%s: start update netns list", probeName)
			ets := nettop.GetAllUniqueNetnsEntity()
			for _, et := range ets {
				if et == nil {
					log.Infof("%s: skip empty entity", probeName)
					continue
				}
				nsHandle, err := et.OpenNsHandle()
				if err != nil {
					log.Infof("%s: failed get netns fd, skip netns fd, err: %v", probeName, err)
					continue
				}
				if nsHandle == 0 {
					log.Infof("%s: invalid nsfd(0), skip empty netns fd", probeName)
					continue
				}
				if _, ok := p.conns[et.GetNetns()]; !ok {
					ctrch := make(chan struct{})
					go func() {
						err = p.startCtListen(ctx, ctrch, nsHandle, et.GetNetns())
						if err != nil {
							log.Infof("%s: failed start worker, err: %v", probeName, err)
							return
						}
					}()
					p.conns[et.GetNetns()] = ctrch
					log.Infof("%s: start worker finished", probeName)
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

func (p *conntrackEventProbe) startCtListen(_ context.Context, ctrch <-chan struct{}, nsHandle netns.NsHandle, nsinum int) error {
	c, err := conntrack.Dial(&netlink.Config{
		NetNS: int(nsHandle),
	})
	defer nsHandle.Close()

	if err != nil {
		log.Infof("%s: failed start conntrack dial, err: %v", probeName, err)
		return err
	}

	log.Infof("%s: start conntrack listen in netns %d", probeName, nsHandle)
	evCh := make(chan conntrack.Event, 1024)
	errCh, err := c.Listen(evCh, 4, append(netfilter.GroupsCT, netfilter.GroupsCTExp...))
	if err != nil {
		log.Infof("%s: failed start conntrack listen, err: %v", probeName, err)
		return err
	}

	for {
		select {
		case <-ctrch:
			log.Infof("%s: conntrack event listen stop", probeName)
			return nil
		case err = <-errCh:
			log.Infof("%s: conntrack event listen stop, err: %v", probeName, err)
			return err
		case event := <-evCh:
			p.sink <- vanishEvent(event, nsinum)
			log.Infof("%s: conntrack event listen got event: %s", probeName, event.String())
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
