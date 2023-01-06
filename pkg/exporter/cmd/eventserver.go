package cmd

import (
	"context"
	"fmt"
	"sync"
	"time"

	lokiwrapper "github.com/alibaba/kubeskoop/pkg/exporter/loki"
	nettop2 "github.com/alibaba/kubeskoop/pkg/exporter/nettop"
	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	proto2 "github.com/alibaba/kubeskoop/pkg/exporter/proto"

	"github.com/google/uuid"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc/peer"
)

type EServer struct {
	proto2.UnimplementedInspectorServer
	probes  map[string]proto2.EventProbe
	cpool   map[string]chan<- proto2.RawEvent
	mtx     sync.Mutex
	ctx     context.Context
	control chan struct{}
	config  EventConfig
}

func NewEServer(ctx context.Context, config EventConfig) *EServer {
	es := &EServer{
		probes:  make(map[string]proto2.EventProbe),
		cpool:   make(map[string]chan<- proto2.RawEvent),
		config:  config,
		mtx:     sync.Mutex{},
		ctx:     ctx,
		control: make(chan struct{}),
	}

	if len(config.Probes) == 0 {
		// if no probes configured, keep loop channel empty
		slog.Ctx(ctx).Info("new eserver with no probe required")
		return es
	}

	for _, p := range config.Probes {
		ep := probe.GetEventProbe(p)
		if ep == nil {
			slog.Ctx(ctx).Info("get event probe nil", "probe", p)
			continue
		}
		es.probes[p] = ep
		go ep.Start(ctx)
		slog.Ctx(ctx).Debug("eserver start", "subject", p)
	}

	// start cache loop
	slog.Ctx(ctx).Debug("new eserver start cache loop")
	go es.dispatcher(ctx, es.control)

	// handle grafana loki ingester preparation
	if config.LokiEnable && config.LokiAddress != "" {
		datach := make(chan proto2.RawEvent)
		ingester, err := lokiwrapper.NewIngester(config.LokiAddress, nettop2.GetNodeName(), datach)
		if err != nil {
			slog.Ctx(ctx).Info("new loki ingester", "err", err, "client", ingester.Name())
		} else {
			es.subscribe(ingester.Name(), datach)
			go ingester.Watch(ctx)
		}

	}

	return es
}

func (e *EServer) WatchEvent(req *proto2.WatchRequest, srv proto2.Inspector_WatchEventServer) error {
	client := getPeerClient(srv.Context())
	datach := make(chan proto2.RawEvent)
	slog.Ctx(e.ctx).Info("watch event income", "client", client)
	e.subscribe(client, datach)
	defer e.unsubscribe(client)

	for evt := range datach {
		resp := &proto2.WatchReply{
			Name: evt.EventType,
			Event: &proto2.Event{
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

func (e *EServer) QueryMetric(ctx context.Context, req *proto2.QueryMetricRequest) (*proto2.QueryMetricResponse, error) {
	res := &proto2.QueryMetricResponse{}
	return res, nil
}

func (e *EServer) subscribe(client string, ch chan<- proto2.RawEvent) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	e.cpool[client] = ch
}

func (e *EServer) unsubscribe(client string) {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	delete(e.cpool, client)
}

func (e *EServer) dispatcher(ctx context.Context, stopc chan struct{}) {
	pbs := e.probes
	receiver := make(chan proto2.RawEvent)
	for p, pb := range pbs {
		err := pb.Register(receiver)
		if err != nil {
			slog.Ctx(ctx).Info("register receiver", "probe", p, "err", err)
			continue
		}
	}

	slog.Ctx(ctx).Debug("dispatcher", "probes", pbs)
	for {
		select {

		case <-stopc:
			slog.Ctx(ctx).Debug("dispatcher exit of sop signal", "probes", pbs)
			return
		case evt := <-receiver:
			err := e.broadcast(evt)
			if err != nil {
				slog.Ctx(ctx).Info("dispatcher broadcast", "err", err, "event", evt)
				continue
			}
		}

	}
}

func (e *EServer) broadcast(evt proto2.RawEvent) error {
	pbs := e.cpool

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

func getEventMetaByNetns(ctx context.Context, netns uint32) *proto2.Meta {
	et, err := nettop2.GetEntityByNetns(int(netns))
	if err != nil {
		slog.Ctx(ctx).Info("nettop get entity", "err", err, "netns", netns)
		return nil
	}

	return &proto2.Meta{
		Pod:       et.GetPodName(),
		Namespace: et.GetPodNamespace(),
		Netns:     fmt.Sprintf("ns%d", netns),
		Node:      nettop2.GetNodeName(),
	}
}
