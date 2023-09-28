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

func NewMetricsServer() (*MetricsServer, error) {

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

func (m *MetricsProbeManager) StartProbe(ctx context.Context, probe probe.MetricsProbe) error {
	log.Infof("start metrics probe %s", probe.Name())
	if err := probe.Start(ctx); err != nil {
		return err
	}
	m.prometheusRegistry.MustRegister(probe)
	return nil
}

func (m *MetricsProbeManager) StopProbe(ctx context.Context, probe probe.MetricsProbe) error {
	log.Infof("stop metrics probe %s", probe.Name())
	if err := probe.Stop(ctx); err != nil {
		return err
	}
	m.prometheusRegistry.Unregister(probe)
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
