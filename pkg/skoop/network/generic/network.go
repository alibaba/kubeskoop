package generic

import (
	"context"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	"github.com/alibaba/kubeskoop/pkg/skoop/collector/manager"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/nodemanager"
	plugin2 "github.com/alibaba/kubeskoop/pkg/skoop/plugin"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/alibaba/kubeskoop/pkg/skoop/skoop"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type flannelNetwork struct {
	plugin           plugin2.Plugin
	diagnostor       skoop.Diagnostor
	collectorManager collector.Manager
	netNodeManager   nodemanager.NetNodeManager
}

func (f *flannelNetwork) Diagnose(ctx *ctx.Context, src model2.Endpoint, dst model2.Endpoint) ([]model2.Suspicion, *model2.PacketPath, error) {
	return f.diagnostor.Diagnose(src, dst, model2.Protocol(ctx.TaskConfig().Protocol))
}

type calicoNetwork struct {
	plugin           plugin2.Plugin
	diagnostor       skoop.Diagnostor
	collectorManager collector.Manager
	netNodeManager   nodemanager.NetNodeManager
}

func (n *calicoNetwork) Diagnose(ctx *ctx.Context, src model2.Endpoint, dst model2.Endpoint) ([]model2.Suspicion, *model2.PacketPath, error) {
	return n.diagnostor.Diagnose(src, dst, model2.Protocol(ctx.TaskConfig().Protocol))
}

func getFlannelCNIMode(ctx *ctx.Context) (plugin2.FlannelBackendType, error) {
	cfg, err := ctx.KubernetesClient().CoreV1().
		ConfigMaps("kube-flannel").Get(context.TODO(), "kube-flannel-cfg", metav1.GetOptions{})
	if err != nil {
		return "", nil
	}

	conf := cfg.Data["net-conf.json"]
	if strings.Contains(conf, "vxlan") {
		return plugin2.FlannelBackendTypeVxlan, nil
	}
	if strings.Contains(conf, "hsot-gw") {
		return plugin2.FlannelBackendTypeHostGW, nil
	}

	return plugin2.FlannelBackendTypeAlloc, nil
}

func NewFlannelNetwork(ctx *ctx.Context) (network.Network, error) {
	cniMode, err := getFlannelCNIMode(ctx)
	if err != nil {
		return nil, err
	}

	serviceProcessor := service.NewKubeProxyServiceProcessor(ctx)
	options := &plugin2.FlannelPluginOptions{
		Bridge:           "cni0",
		Interface:        "eth0",
		PodMTU:           1500,
		IPMasq:           true,
		ClusterCIDR:      ctx.ClusterConfig().ClusterCIDR,
		CNIMode:          cniMode,
		ServiceProcessor: serviceProcessor,
	}

	if cniMode == plugin2.FlannelBackendTypeVxlan {
		options.PodMTU = 1450
	}

	plgn, err := plugin2.NewFlannelPlugin(ctx, options)
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

	networkPolicy, err := plugin2.NewNetworkPolicy(false, false, ctx.ClusterConfig().IPCache, ctx.KubernetesClient(), serviceProcessor)
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
	options := &plugin2.CalicoPluginOptions{
		Interface:        "eth0",
		PodMTU:           1500,
		IPIPPodMTU:       1480,
		ServiceProcessor: serviceProcessor,
	}

	plgn, err := plugin2.NewCalicoPlugin(ctx, options)
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

	networkPolicy, err := plugin2.NewNetworkPolicy(true, false, ctx.ClusterConfig().IPCache, ctx.KubernetesClient(), serviceProcessor)
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
