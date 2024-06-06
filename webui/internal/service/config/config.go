package config

import (
	"fmt"
	"net/url"

	"github.com/kubeskoop/webconsole/internal/config"
)

const (
	metricsDashboardURL = "/d/%s/%s?orgId=1&theme=light&from=now-15m&to=now&refresh=10s"

	podDashboardUID   = "ddn87hjw7bdhcd"
	podDashboardName  = "skoop-exporter-pods"
	nodeDashboardUID  = "bdn87gdt1emtcb"
	nodeDashboardName = "skoop-exporter-nodes"
)

type DashboardConfig struct {
	PodDashboardURL  string
	NodeDashboardURL string
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
			cfg.PodDashboardURL = fmt.Sprintf("/grafana%s", fmt.Sprintf(metricsDashboardURL, podDashboardUID, podDashboardName))
			cfg.NodeDashboardURL = fmt.Sprintf("/grafana%s", fmt.Sprintf(metricsDashboardURL, nodeDashboardUID, nodeDashboardName))
		} else {
			u, err := url.JoinPath(config.Global.Grafana.Endpoint, fmt.Sprintf(metricsDashboardURL, podDashboardUID, podDashboardName))
			if err != nil {
				return nil, err
			}
			cfg.PodDashboardURL = u

			u, err = url.JoinPath(config.Global.Grafana.Endpoint, fmt.Sprintf(metricsDashboardURL, nodeDashboardUID, nodeDashboardName))
			if err != nil {
				return nil, err
			}
			cfg.NodeDashboardURL = u
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
