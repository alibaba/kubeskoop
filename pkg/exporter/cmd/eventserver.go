package cmd

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"sync"
	"time"

	lokiwrapper "github.com/alibaba/kubeskoop/pkg/exporter/loki"
	"github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/google/uuid"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc/peer"
)

type EServer struct {
	proto.UnimplementedInspectorServer
	probes       map[string]proto.EventProbe
	subscribers  map[string]chan<- proto.RawEvent
	mtx          sync.Mutex
	ctx          context.Context
	control      chan struct{}
	config       EventConfig
	eventChan    chan proto.RawEvent
	lokiDatach   chan proto.RawEvent
	lokiIngester *lokiwrapper.Ingester
}

func NewEServer(ctx context.Context, config EventConfig) *EServer {
	es := &EServer{
		probes:      make(map[string]proto.EventProbe),
		subscribers: make(map[string]chan<- proto.RawEvent),
		config:      config,
		mtx:         sync.Mutex{},
		ctx:         ctx,
		control:     make(chan struct{}),
		eventChan:   make(chan proto.RawEvent),
	}

	for _, p := range config.Probes {
		ep := probe.GetEventProbe(p)
		if ep == nil {
			slog.Ctx(ctx).Info("get event probe nil", "probe", p)
			continue
		}
		es.probes[p] = ep
		err := ep.Register(es.eventChan)
		if err != nil {
			slog.Ctx(ctx).Warn("probe register failed", "probe", p)
			continue
		}
		go ep.Start(ctx, proto.ProbeTypeEvent)
		slog.Ctx(ctx).Debug("eserver start", "subject", p)
	}

	// start cache loop
	slog.Ctx(ctx).Debug("new eserver start dispatch loop")
	go es.dispatcher(ctx, es.control)

	err := es.enableLoki()
	if err != nil {
		slog.Ctx(ctx).Warn("enable loki failed", "err", err)
	}
	return es
}

func (e *EServer) enableLoki() error {
	if e.lokiIngester != nil {
		return nil
	}

	// handle grafana loki ingester preparation
	if e.config.LokiEnable && e.config.LokiAddress != "" {
		slog.Ctx(e.ctx).Debug("enabling loki ingester", "address", e.config.LokiAddress)
		datach := make(chan proto.RawEvent)
		ingester, err := lokiwrapper.NewLokiIngester(e.ctx, e.config.LokiAddress, nettop.GetNodeName())
		if err != nil {
			slog.Ctx(e.ctx).Info("new loki ingester", "err", err, "client", ingester.Name())
		} else {
			e.subscribe(ingester.Name(), datach)
			go ingester.Watch(e.ctx, datach)
		}
		e.lokiDatach = datach
		e.lokiIngester = ingester
	}

	return nil
}

func (e *EServer) disableLoki() error {
	if e.lokiIngester == nil {
		return nil
	}

	slog.Ctx(e.ctx).Debug("disabling loki ingester")
	e.unsubscribe(e.lokiIngester.Name())

	err := e.lokiIngester.Close()
	if err != nil {
		return err
	}

	close(e.lokiDatach)
	e.lokiIngester = nil
	e.lokiDatach = nil
	return nil
}

func (e *EServer) Reload(config EventConfig) error {
	enabled := lo.Keys(e.probes)
	toClose, toStart := lo.Difference(enabled, config.Probes)
	slog.Ctx(e.ctx).Info("reload event probes", "close", toClose, "enable", toStart)

	for _, n := range toClose {
		p, ok := e.probes[n]
		if !ok {
			slog.Ctx(e.ctx).Warn("probe not found in enabled probes, skip.", "probe", n)
			continue
		}

		err := p.Close(proto.ProbeTypeEvent)
		if err != nil {
			slog.Ctx(e.ctx).Warn("close probe error", "probe", n, "err", err)
			continue
		}

		// clear event channel
		err = p.Register(nil)
		if err != nil {
			slog.Ctx(e.ctx).Warn("unregister probe error", "probe", n, "err", err)
			continue
		}

		delete(e.probes, n)
	}

	for _, n := range toStart {
		p := probe.GetEventProbe(n)
		if p == nil {
			slog.Ctx(e.ctx).Info("get event probe nil", "probe", p)
			continue
		}
		e.probes[n] = p
		go p.Start(e.ctx, proto.ProbeTypeEvent)
		slog.Ctx(e.ctx).Debug("eserver start", "subject", p)

		err := p.Register(e.eventChan)
		if err != nil {
			slog.Ctx(e.ctx).Info("register receiver", "probe", p, "err", err)
			continue
		}
	}

	e.config = config

	if config.LokiEnable {
		if err := e.enableLoki(); err != nil {
			slog.Ctx(e.ctx).Warn("enable loki error", "err", err)
		}
	} else {
		if err := e.disableLoki(); err != nil {
			slog.Ctx(e.ctx).Warn("disable loki error", "err", err)
		}
	}

	return nil
}

func (e *EServer) WatchEvent(_ *proto.WatchRequest, srv proto.Inspector_WatchEventServer) error {
	client := getPeerClient(srv.Context())
	datach := make(chan proto.RawEvent)
	slog.Ctx(e.ctx).Info("watch event income", "client", client)
	e.subscribe(client, datach)
	defer e.unsubscribe(client)

	for evt := range datach {
		resp := &proto.WatchReply{
			Name: evt.EventType,
			Event: &proto.Event{
				Name:  evt.EventType,
				Value: evt.EventBody,
				Meta:  getEventMetaByNetns(e.ctx, evt.Netns),
			},
		}
		err := srv.Send(resp)
		if err != nil {
			slog.Ctx(e.ctx).Warn("watch event", "err", err, "client", client)
			return err
		}
	}

	return nil
}

func (e *EServer) QueryMetric(_ context.Context, _ *proto.QueryMetricRequest) (*proto.QueryMetricResponse, error) {
	res := &proto.QueryMetricResponse{}
	return res, nil
}

func (e *EServer) subscribe(client string, ch chan<- proto.RawEvent) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	e.subscribers[client] = ch
}

func (e *EServer) unsubscribe(client string) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	delete(e.subscribers, client)
}

func (e *EServer) dispatcher(ctx context.Context, stopc chan struct{}) {
	for {
		select {
		case <-stopc:
			slog.Ctx(ctx).Debug("event dispatcher exited because of stop signal")
			return
		case evt := <-e.eventChan:
			err := e.broadcast(evt)
			if err != nil {
				slog.Ctx(ctx).Info("dispatcher broadcast", "err", err, "event", evt)
				continue
			}
		}

	}
}

func (e *EServer) broadcast(evt proto.RawEvent) error {
	pbs := e.subscribers

	ctx, cancelf := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancelf()
	workdone := make(chan struct{})
	go func(done chan struct{}) {
		for client, c := range pbs {
			c <- evt
			slog.Ctx(e.ctx).Debug("broadcast event", "client", client, "event", evt.EventType)
		}

		done <- struct{}{}
	}(workdone)

	if e.config.InfoToLog {
		slog.Ctx(e.ctx).Warn("broadcast event", "type", evt.EventType, "body", evt.EventBody, "netns", evt.Netns)
	}

	select {
	case <-ctx.Done():
		slog.Ctx(e.ctx).Info("broadcast event stuck", "event", evt.EventType)
		return context.DeadlineExceeded
	case <-workdone:
		slog.Ctx(e.ctx).Info("broadcast event", "event", evt.EventType, "info", evt.EventBody)
	}

	return nil
}

func getPeerClient(ctx context.Context) string {
	var clientid string
	pr, ok := peer.FromContext(ctx)
	if ok {
		clientid = pr.Addr.String()
	} else {
		clientid = uuid.New().String()
	}

	return clientid
}

func getEventMetaByNetns(ctx context.Context, netns uint32) *proto.Meta {
	et, err := nettop.GetEntityByNetns(int(netns))
	if err != nil {
		slog.Ctx(ctx).Info("nettop get entity", "err", err, "netns", netns)
		return nil
	}

	return &proto.Meta{
		Pod:       et.GetPodName(),
		Namespace: et.GetPodNamespace(),
		Netns:     fmt.Sprintf("ns%d", netns),
		Node:      nettop.GetNodeName(),
	}
}
