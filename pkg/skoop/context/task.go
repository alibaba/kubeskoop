package context

import (
	"fmt"
	"net"

	"golang.org/x/exp/slices"

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
	if tc.Source == "" || tc.Destination.Address == "" {
		return fmt.Errorf("source or destination address cannot be empty")
	}

	if tc.Destination.Port <= 0 || tc.Destination.Port > 65535 {
		return fmt.Errorf("a valid destination port should be provided")
	}

	ip := net.ParseIP(tc.Source)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("source address should be a valid IPv4 address")
	}

	ip = net.ParseIP(tc.Destination.Address)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("destination address should be a valid IPv4 address")
	}

	if !slices.Contains([]string{"tcp", "udp"}, tc.Protocol) {
		return fmt.Errorf("protocol should be tcp,udp")
	}

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
