package cmd

import (
	"context"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/sink"
	log "github.com/sirupsen/logrus"
)

type EventServer struct {
	*DynamicProbeServer[probe.EventProbe]
}

func NewEventServer(sinks []sink.Sink) (*EventServer, error) {
	var sinkWrappers []*sinkWrapper

	done := make(chan struct{})

	for _, s := range sinks {
		sinkWrappers = append(sinkWrappers, &sinkWrapper{
			ch:   make(chan *probe.Event, 1024),
			s:    s,
			done: done,
		})
	}

	probeManager := &EventProbeManager{
		sinks:    sinkWrappers,
		sinkChan: make(chan *probe.Event),
		done:     done,
	}

	return &EventServer{
		DynamicProbeServer: NewDynamicProbeServer[probe.EventProbe](probeManager),
	}, nil
}

func (s *EventServer) Start(ctx context.Context, probeConfig []ProbeConfig) error {
	s.probeManager.(*EventProbeManager).start()
	return s.DynamicProbeServer.Start(ctx, probeConfig)
}

func (s *EventServer) Stop(ctx context.Context) error {
	if err := s.DynamicProbeServer.Stop(ctx); err != nil {
		return err
	}
	s.probeManager.(*EventProbeManager).stop()
	return nil
}

type EventProbeManager struct {
	sinkChan chan *probe.Event
	sinks    []*sinkWrapper
	done     chan struct{}
}

type sinkWrapper struct {
	ch   chan *probe.Event
	s    sink.Sink
	done chan struct{}
}

func (m *EventProbeManager) stop() {
	log.Infof("probe manager stopped")
	close(m.done)
}

func consume(sw *sinkWrapper) {
loop:
	for {
		select {
		case evt := <-sw.ch:
			if err := sw.s.Write(evt); err != nil {
				log.Errorf("error sink evt %s", err)
			}
		case <-sw.done:
			break loop
		}
	}
}

func (m *EventProbeManager) start() {
	for _, s := range m.sinks {
		go consume(s)
	}

	go func() {
	loop:
		for {
			select {
			case evt := <-m.sinkChan:
				for _, sw := range m.sinks {
					select {
					case sw.ch <- evt:
						break
					default:
						log.Errorf("%s is blocked, discard event.", sw.s)
					}
				}
			case <-m.done:
				break loop
			}
		}
	}()
}

func (m *EventProbeManager) CreateProbe(config ProbeConfig) (probe.EventProbe, error) {
	return probe.CreateEventProbe(config.Name, m.sinkChan, config.Args)
}

func (m *EventProbeManager) StartProbe(ctx context.Context, probe probe.EventProbe) error {
	return probe.Start(ctx)
}

func (m *EventProbeManager) StopProbe(ctx context.Context, probe probe.EventProbe) error {
	return probe.Stop(ctx)
}

var _ ProbeManager[probe.MetricsProbe] = &MetricsProbeManager{}
