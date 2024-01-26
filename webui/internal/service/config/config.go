package config

import (
	"fmt"
	"net/url"

	"github.com/kubeskoop/webconsole/internal/config"
)

const (
	dashboardUID        = "PtAs82D4k"
	metricsDashboardURL = "/d/%s/skoop-exporter?orgId=1&theme=light&from=now-15m&to=now&refresh=10s"
)

type DashboardConfig struct {
	MetricsURL string
	EventURL   string
	FlowURL    string
}

var Service Svc

type Svc interface {
	SetDashboardConfig(config DashboardConfig) error
	GetDashboardConfig() (DashboardConfig, error)
}

func init() {
	svc, err := newDefaultService()
	if err != nil {
		panic(err)
	}
	Service = svc
}

type defaultService struct {
	config DashboardConfig
}

func newDefaultService() (*defaultService, error) {
	cfg := DashboardConfig{}
	if config.Global.Grafana.Endpoint != "" {
		if config.Global.Grafana.Proxy {
			cfg.MetricsURL = fmt.Sprintf("/grafana%s", fmt.Sprintf(metricsDashboardURL, dashboardUID))
		} else {
			u, err := url.JoinPath(config.Global.Grafana.Endpoint, fmt.Sprintf(metricsDashboardURL, dashboardUID))
			if err != nil {
				return nil, err
			}
			cfg.MetricsURL = u
		}
	}

	return &defaultService{
		config: cfg,
	}, nil
}

func (d *defaultService) SetDashboardConfig(config DashboardConfig) error {
	d.config = config
	return nil
}

func (d *defaultService) GetDashboardConfig() (DashboardConfig, error) {
	return d.config, nil
}
