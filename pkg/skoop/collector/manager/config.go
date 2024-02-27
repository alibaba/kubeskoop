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
	context.RegisterConfigBinder("Simple pod collector", &Config.SimplePodCollectorConfig)
}

type SimplePodCollectorConfig struct {
	Image                string
	CollectorNamespace   string
	RuntimeAPIAddress    string
	WaitInterval         time.Duration
	WaitTimeout          time.Duration
	PreserveCollectorPod bool
}

func (cc *SimplePodCollectorConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&cc.Image, "collector-image", "", "kubeskoop/agent:v1.0.0", "Image used for collector.")
	fs.StringVarP(&cc.CollectorNamespace, "collector-namespace", "", "skoop", "Namespace where collector pods in.")
	fs.StringVarP(&cc.RuntimeAPIAddress, "collector-cri-address", "", "", "Runtime CRI API endpoint address.")
	fs.DurationVarP(&cc.WaitInterval, "collector-pod-wait-interval", "", 2*time.Second, "Collector pod running check interval.")
	fs.DurationVarP(&cc.WaitTimeout, "collector-pod-wait-timeout", "", 120*time.Second, "Collector pod running check timeout.")
	fs.BoolVarP(&cc.PreserveCollectorPod, "preserve-collector-pod", "", false, "Preserve collector pod after diagnosis complete.")
}

func (cc *SimplePodCollectorConfig) Validate() error {
	return nil
}
