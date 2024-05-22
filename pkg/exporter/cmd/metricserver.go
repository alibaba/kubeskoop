package cmd

import (
	"context"
	"net/http"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func newMetricsServer() (*MetricsServer, error) {

	r := prometheus.NewRegistry()
	handler := promhttp.HandlerFor(prometheus.Gatherers{
		r,
	}, promhttp.HandlerOpts{})

	probeManager := &MetricsProbeManager{
		prometheusRegistry: r,
	}

	return &MetricsServer{
		DynamicProbeServer: NewDynamicProbeServer[probe.MetricsProbe](probeManager),
		httpHandler:        handler,
	}, nil
}

type MetricsProbeManager struct {
	prometheusRegistry *prometheus.Registry
}

func (m *MetricsProbeManager) CreateProbe(config ProbeConfig) (probe.MetricsProbe, error) {
	log.Infof("create metrics probe %s with args %s", config.Name, util.ToJSONString(config.Args))
	return probe.CreateMetricsProbe(config.Name, config.Args)
}

func (m *MetricsProbeManager) StartProbe(ctx context.Context, p probe.MetricsProbe) error {
	log.Infof("start metrics probe %s", p.Name())
	if err := p.Start(ctx); err != nil {
		return err
	}
	m.prometheusRegistry.MustRegister(p)
	return nil
}

func (m *MetricsProbeManager) StopProbe(ctx context.Context, p probe.MetricsProbe) error {
	log.Infof("stop metrics probe %s", p.Name())

	state := p.State()
	if state == probe.ProbeStateStopped || state == probe.ProbeStateStopping || state == probe.ProbeStateFailed {
		return nil
	}

	if err := p.Stop(ctx); err != nil {
		return err
	}
	m.prometheusRegistry.Unregister(p)
	return nil
}

var _ ProbeManager[probe.MetricsProbe] = &MetricsProbeManager{}

type MetricsServer struct {
	*DynamicProbeServer[probe.MetricsProbe]
	httpHandler http.Handler
}

func (s *MetricsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.httpHandler.ServeHTTP(w, r)
}
