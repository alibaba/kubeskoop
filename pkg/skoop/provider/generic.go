package provider

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/network/generic"
)

type genericProvider struct {
}

func (g genericProvider) CreateNetwork(ctx *context.Context) (network.Network, error) {
	switch ctx.ClusterConfig().NetworkPlugin {
	case context.NetworkPluginFlannel:
		return generic.NewFlannelNetwork(ctx)
	case context.NetworkPluginCalico:
		return generic.NewCalicoNetwork(ctx)
	default:
		return nil, fmt.Errorf("not support cni type %q", ctx.ClusterConfig().NetworkPlugin)
	}
}
