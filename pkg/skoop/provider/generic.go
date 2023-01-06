package provider

import (
	"fmt"

	context2 "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/network/generic"
)

type genericProvider struct {
}

func (g genericProvider) CreateNetwork(ctx *context2.Context) (network.Network, error) {
	switch ctx.ClusterConfig().NetworkPluginName {
	case context2.NetworkPluginFlannel:
		return generic.NewFlannelNetwork(ctx)
	case context2.NetworkPluginCalico:
		return generic.NewCalicoNetwork(ctx)
	default:
		return nil, fmt.Errorf("not support cni type %s", ctx.ClusterConfig().NetworkPluginName)
	}
}
