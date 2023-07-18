package context

import (
	"sync"

	cliflag "k8s.io/component-base/cli/flag"

	"github.com/spf13/pflag"
)

const (
	taskConfigKey           = "task_config"
	clusterConfigKey        = "cluster_config"
	kubernetesClientKey     = "kubernetes_client_config"
	kubernetesRestConfigKey = "kubernetes_rest_config"
	uiConfigKey             = "ui_config"
	miscConfigKey           = "misc_config"
)

type ConfigBinder interface {
	BindFlags(fs *pflag.FlagSet)
	Validate() error
}

type NamedConfigBinder struct {
	Name   string
	Binder ConfigBinder
}

var binders []NamedConfigBinder

func RegisterConfigBinder(name string, binder ConfigBinder) {
	binders = append(binders, NamedConfigBinder{
		Name:   name,
		Binder: binder,
	})
}

func init() {
	tc := &TaskConfig{}
	cc := &ClusterConfig{}
	uc := &UIConfig{}
	mc := &MiscConfig{}
	RegisterConfigBinder("Diagnose task", tc)
	RegisterConfigBinder("Cluster config", cc)
	RegisterConfigBinder("UI config", uc)
	RegisterConfigBinder("Miscellaneous config", mc)

	SkoopContext.Ctx.Store(taskConfigKey, tc)
	SkoopContext.Ctx.Store(clusterConfigKey, cc)
	SkoopContext.Ctx.Store(uiConfigKey, uc)
	SkoopContext.Ctx.Store(miscConfigKey, mc)
}

var SkoopContext = &Context{
	Ctx: &sync.Map{},
}

type Context struct {
	Ctx *sync.Map
}

func (c *Context) BindFlags(fs *pflag.FlagSet) {
	for _, b := range binders {
		b.Binder.BindFlags(fs)
	}
}

func (c *Context) BindNamedFlags(fss *cliflag.NamedFlagSets) {
	for _, b := range binders {
		fs := fss.FlagSet(b.Name)
		b.Binder.BindFlags(fs)
	}
}

func (c *Context) Validate() error {
	for _, b := range binders {
		err := b.Binder.Validate()
		if err != nil {
			return err
		}
	}
	return nil
}
