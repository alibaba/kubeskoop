package provider

import (
	"fmt"

	context2 "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/network/aliyun"
)

type aliyunProvider struct {
}

func (p aliyunProvider) CreateNetwork(ctx *context2.Context) (network.Network, error) {
	// 配置infrashim
	// 判断cni类型
	// 配置networknodemanager
	// image地址
	switch ctx.ClusterConfig().NetworkPluginName {
	case context2.NetworkPluginFlannel:
		return aliyun.NewFlannelNetwork(ctx)
	case context2.NetworkPluginCalico:
		return aliyun.NewCalicoNetwork(ctx)
	default:
		return nil, fmt.Errorf("not support cni type %s", ctx.ClusterConfig().NetworkPluginName)
	}
}
