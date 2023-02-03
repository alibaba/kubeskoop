package manager

import (
	"time"

	"github.com/alibaba/kubeskoop/pkg/skoop/context"

	"github.com/spf13/pflag"
)

type ConfigSet struct {
	SimplePodCollectorConfig SimplePodCollectorConfig
}

var Config = &ConfigSet{}

func init() {
	context.RegisterConfigBinder(&Config.SimplePodCollectorConfig)
}

type SimplePodCollectorConfig struct {
	Image              string
	CollectorNamespace string
	WaitInterval       time.Duration
	WaitTimeout        time.Duration
}

func (cc *SimplePodCollectorConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&cc.Image, "collector-image", "", "kubeskoop/kubeskoop:v0.1.0", "Image used for collector.")
	fs.StringVarP(&cc.CollectorNamespace, "collector-namespace", "", "skoop", "Namespace where collector pods in.")
	fs.DurationVarP(&cc.WaitInterval, "collector-pod-wait-interval", "", 2*time.Second, "Collector pod running check interval.")
	fs.DurationVarP(&cc.WaitTimeout, "collector-pod-wait-timeout", "", 120*time.Second, "Collector pod running check timeout.")
}

func (cc *SimplePodCollectorConfig) Validate() error {
	return nil
}
