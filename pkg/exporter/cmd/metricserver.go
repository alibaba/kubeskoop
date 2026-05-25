package cmd

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

func newMetricsServer(config MetricsConfig) (*MetricsServer, error) {

	r := prometheus.NewRegistry()

	probeManager := &MetricsProbeManager{
		prometheusRegistry: r,
	}

	server := &MetricsServer{
		DynamicProbeServer: NewDynamicProbeServer[probe.MetricsProbe](probeManager),
		prometheusRegistry: r,
	}
	server.SetDisableCompression(config.DisableCompression)
	return server, nil
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
	prometheusRegistry *prometheus.Registry
	httpHandler        atomic.Value
}

func (s *MetricsServer) SetDisableCompression(disable bool) {
	handler := promhttp.HandlerFor(prometheus.Gatherers{
		s.prometheusRegistry,
	}, promhttp.HandlerOpts{DisableCompression: disable})
	s.httpHandler.Store(handler)
}

func (s *MetricsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.httpHandler.Load().(http.Handler).ServeHTTP(w, r)
}
