package config

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
	return &defaultService{
		config: DashboardConfig{},
	}, nil
}

func (d *defaultService) SetDashboardConfig(config DashboardConfig) error {
	d.config = config
	return nil
}

func (d *defaultService) GetDashboardConfig() (DashboardConfig, error) {
	return d.config, nil
}
