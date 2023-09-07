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
	probeManager := &EventProbeManager{
		sinks:    sinks,
		sinkChan: make(chan *probe.Event),
		done:     make(chan struct{}),
	}

	return &EventServer{
		DynamicProbeServer: NewDynamicProbeServer[probe.EventProbe](probeManager),
	}, nil
}

func (s *EventServer) Start(ctx context.Context, probeConfig []ProbeConfig) error {
	go s.probeManager.(*EventProbeManager).start()
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
	sinks    []sink.Sink
	done     chan struct{}
}

func (m *EventProbeManager) stop() {
	log.Infof("probe manager stopped")
	close(m.done)
}

func (m *EventProbeManager) start() {
	for {
		select {
		case evt := <-m.sinkChan:
			for _, sink := range m.sinks {
				//TODO be concurrency
				if err := sink.Write(evt); err != nil {
					log.Errorf("error sink evt %s", err)
				}
			}
		case <-m.done:
			break
		}
	}
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
