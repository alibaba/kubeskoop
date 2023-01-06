package context

import (
	"sync"

	"github.com/spf13/pflag"
)

const (
	taskConfigKey           = "task_config"
	clusterConfigKey        = "cluster_config"
	kubernetesClientKey     = "kubernetes_client_config"
	kubernetesRestConfigKey = "kubernetes_rest_config"
	uiConfigKey             = "ui_config"
)

type ConfigBinder interface {
	BindFlags(fs *pflag.FlagSet)
	Validate() error
}

var binders []ConfigBinder

func RegisterConfigBinder(binder ConfigBinder) {
	binders = append(binders, binder)
}

func init() {
	tc := &TaskConfig{}
	cc := &ClusterConfig{}
	uc := &UIConfig{}
	RegisterConfigBinder(tc)
	RegisterConfigBinder(cc)
	RegisterConfigBinder(uc)

	SkoopContext.Ctx.Store(taskConfigKey, tc)
	SkoopContext.Ctx.Store(clusterConfigKey, cc)
	SkoopContext.Ctx.Store(uiConfigKey, uc)
}

var SkoopContext = &Context{
	Ctx: &sync.Map{},
}

type Context struct {
	Ctx *sync.Map
}

func (c *Context) BindFlags(fs *pflag.FlagSet) {
	for _, b := range binders {
		b.BindFlags(fs)
	}
}

func (c *Context) Validate() error {
	for _, b := range binders {
		err := b.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}
