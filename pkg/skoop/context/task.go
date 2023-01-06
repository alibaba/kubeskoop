package context

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	"github.com/spf13/pflag"
)

type TaskConfig struct {
	Source      string
	Destination struct {
		Address string
		Port    uint16
	}
	SourceEndpoint model.Endpoint
	DstEndpoint    model.Endpoint
	Protocol       string
}

func (tc *TaskConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&tc.Source, "src", "s", "", "Source address for the network problem.")
	fs.StringVarP(&tc.Destination.Address, "dst", "d", "", "Destination address for the network problem.")
	fs.Uint16VarP(&tc.Destination.Port, "dport", "p", 0, "Destination port for the network problem.")
	fs.StringVar(&tc.Protocol, "protocol", string(model.TCP), "Protocol for the network problem.")
}

func (tc *TaskConfig) Validate() error {
	return nil
}

func (c *Context) TaskConfig() *TaskConfig {
	taskConfig, _ := c.Ctx.Load(taskConfigKey)
	return taskConfig.(*TaskConfig)
}

func (c *Context) BuildTask() error {
	if c.ClusterConfig().IPCache == nil {
		return fmt.Errorf("ip cache is not initialized")
	}
	srcType, err := c.ClusterConfig().IPCache.GetIPType(c.TaskConfig().Source)
	if err != nil {
		return err
	}
	c.TaskConfig().SourceEndpoint = model.Endpoint{
		IP:   c.TaskConfig().Source,
		Type: srcType,
	}
	dstType, err := c.ClusterConfig().IPCache.GetIPType(c.TaskConfig().Destination.Address)
	if err != nil {
		return err
	}
	c.TaskConfig().DstEndpoint = model.Endpoint{
		IP:   c.TaskConfig().Destination.Address,
		Type: dstType,
		Port: c.TaskConfig().Destination.Port,
	}
	return nil
}
