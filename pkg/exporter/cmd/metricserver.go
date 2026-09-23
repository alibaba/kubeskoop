package cmd

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/probe"
	"github.com/alibaba/kubeskoop/pkg/exporter/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

const metricsWriteTimeout = 30 * time.Second

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
	writeTimeout       time.Duration
}

func (s *MetricsServer) SetDisableCompression(disable bool) {
	handler := promhttp.HandlerFor(prometheus.Gatherers{
		s.prometheusRegistry,
	}, promhttp.HandlerOpts{DisableCompression: disable})
	s.httpHandler.Store(handler)
}

func (s *MetricsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Bound writes on this response only. A write deadline does not interrupt
	// Gather, and concurrent scrapes are handled independently.
	// Keep other endpoints, such as long-running CPU profiles, unaffected.
	timeout := s.writeTimeout
	if timeout <= 0 {
		timeout = metricsWriteTimeout
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		log.Debugf("cannot set metrics response write deadline: %v", err)
	}

	s.httpHandler.Load().(http.Handler).ServeHTTP(w, r)
}
