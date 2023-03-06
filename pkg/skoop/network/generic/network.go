package generic

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	"github.com/alibaba/kubeskoop/pkg/skoop/collector/manager"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/nodemanager"
	"github.com/alibaba/kubeskoop/pkg/skoop/plugin"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/alibaba/kubeskoop/pkg/skoop/skoop"
)

type flannelNetwork struct {
	plugin           plugin.Plugin
	diagnostor       skoop.Diagnostor
	collectorManager collector.Manager
	netNodeManager   nodemanager.NetNodeManager
}

func (f *flannelNetwork) Diagnose(ctx *ctx.Context, src model.Endpoint, dst model.Endpoint) ([]model.Suspicion, *model.PacketPath, error) {
	return f.diagnostor.Diagnose(src, dst, model.Protocol(ctx.TaskConfig().Protocol))
}

type calicoNetwork struct {
	plugin           plugin.Plugin
	diagnostor       skoop.Diagnostor
	collectorManager collector.Manager
	netNodeManager   nodemanager.NetNodeManager
}

func (n *calicoNetwork) Diagnose(ctx *ctx.Context, src model.Endpoint, dst model.Endpoint) ([]model.Suspicion, *model.PacketPath, error) {
	return n.diagnostor.Diagnose(src, dst, model.Protocol(ctx.TaskConfig().Protocol))
}

func NewFlannelNetwork(ctx *ctx.Context) (network.Network, error) {
	serviceProcessor := service.NewKubeProxyServiceProcessor(ctx)

	plgn, err := plugin.NewFlannelPlugin(ctx, serviceProcessor, nil)
	if err != nil {
		return nil, err
	}

	collectorManager, err := manager.NewSimplePodCollectorManager(ctx)
	if err != nil {
		return nil, err
	}

	netNodeManager, err := nodemanager.NewNetNodeManager(ctx, plgn, collectorManager)
	if err != nil {
		return nil, err
	}

	networkPolicy, err := plugin.NewNetworkPolicy(false, false, ctx.ClusterConfig().IPCache, ctx.KubernetesClient(), serviceProcessor)
	if err != nil {
		return nil, err
	}

	diagnostor, err := skoop.NewDefaultDiagnostor(ctx, netNodeManager, networkPolicy)
	if err != nil {
		return nil, err
	}

	return &flannelNetwork{
		plugin:           plgn,
		diagnostor:       diagnostor,
		collectorManager: collectorManager,
		netNodeManager:   netNodeManager,
	}, nil
}

func NewCalicoNetwork(ctx *ctx.Context) (network.Network, error) {
	serviceProcessor := service.NewKubeProxyServiceProcessor(ctx)

	plgn, err := plugin.NewCalicoPlugin(ctx, serviceProcessor, nil)
	if err != nil {
		return nil, err
	}

	collectorManager, err := manager.NewSimplePodCollectorManager(ctx)
	if err != nil {
		return nil, err
	}

	netNodeManager, err := nodemanager.NewNetNodeManager(ctx, plgn, collectorManager)
	if err != nil {
		return nil, err
	}

	networkPolicy, err := plugin.NewNetworkPolicy(true, false, ctx.ClusterConfig().IPCache, ctx.KubernetesClient(), serviceProcessor)
	if err != nil {
		return nil, err
	}

	diagnostor, err := skoop.NewDefaultDiagnostor(ctx, netNodeManager, networkPolicy)
	if err != nil {
		return nil, err
	}

	return &calicoNetwork{
		plugin:           plgn,
		diagnostor:       diagnostor,
		collectorManager: collectorManager,
		netNodeManager:   netNodeManager,
	}, nil
}
