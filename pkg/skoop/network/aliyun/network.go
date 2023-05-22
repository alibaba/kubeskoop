package aliyun

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	"github.com/alibaba/kubeskoop/pkg/skoop/collector/manager"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/infra/aliyun"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/nodemanager"
	"github.com/alibaba/kubeskoop/pkg/skoop/plugin"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/alibaba/kubeskoop/pkg/skoop/skoop"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type flannelNetwork struct {
	plugin           plugin.Plugin
	diagnostor       skoop.Diagnostor
	collectorManager collector.Manager
	netNodeManager   nodemanager.NetNodeManager
}

type calicoNetwork struct {
	plugin           plugin.Plugin
	diagnostor       skoop.Diagnostor
	collectorManager collector.Manager
	netNodeManager   nodemanager.NetNodeManager
}

type terwayNetwork struct {
}

func getRegionAndInstanceID(ctx *ctx.Context) (string, string, error) {
	nodes, err := ctx.KubernetesClient().CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", "", err
	}

	var region, instance string
	for _, node := range nodes.Items {
		if r, ok := node.Labels["topology.kubernetes.io/region"]; ok {
			region = r
		} else if r, ok := node.Labels["failure-domain.beta.kubernetes.io/region"]; ok {
			region = r
		}

		providerID := strings.Split(node.Spec.ProviderID, ".")
		if len(providerID) == 2 {
			instance = providerID[1]
		}

		if region != "" && instance != "" {
			break
		}
	}

	klog.V(3).Infof("Found region %q, instance %q", region, instance)
	if region == "" || instance == "" {
		return "", "", fmt.Errorf("cannot find aliyun region or instance id in cluster")
	}

	return region, instance, nil
}

func buildCloudManager(_ *ctx.Context, region, instanceOfCluster string) (*aliyun.CloudManager, error) {
	options := &aliyun.CloudManagerOptions{
		Region:            region,
		AccessKeyID:       aliyun.Config.AccessKeyID,
		AccessKeySecret:   aliyun.Config.AccessKeySecret,
		SecurityToken:     aliyun.Config.SecurityToken,
		InstanceOfCluster: instanceOfCluster,
	}

	return aliyun.NewCloudManager(options)
}

func buildNetNodeManager(ctx *ctx.Context, plgn plugin.Plugin, infraShim network.InfraShim, serviceProcessor service.Processor, collectorManager collector.Manager) (nodemanager.NetNodeManager, error) {
	aliyunNetNodeManager := &netNodeManager{
		infraShim:  infraShim,
		ipCache:    ctx.ClusterConfig().IPCache,
		processor:  serviceProcessor,
		pluginName: ctx.ClusterConfig().NetworkPlugin,
	}
	return nodemanager.NewNetNodeManagerWithParent(ctx, aliyunNetNodeManager, plgn, collectorManager)
}

func NewFlannelNetwork(ctx *ctx.Context) (network.Network, error) {
	region, instance, err := getRegionAndInstanceID(ctx)
	if err != nil {
		return nil, err
	}

	cloudManager, err := buildCloudManager(ctx, region, instance)
	if err != nil {
		return nil, err
	}

	infraShim, err := NewInfraShim(cloudManager)
	if err != nil {
		return nil, err
	}

	serviceProcessor := service.NewKubeProxyServiceProcessor(ctx)

	plgn, err := plugin.NewFlannelPlugin(ctx, serviceProcessor, infraShim)
	if err != nil {
		return nil, err
	}

	collectorManager, err := manager.NewSimplePodCollectorManager(ctx)
	if err != nil {
		return nil, err
	}

	networkPolicy, err := plugin.NewNetworkPolicy(false, false, ctx.ClusterConfig().IPCache, ctx.KubernetesClient(), serviceProcessor)
	if err != nil {
		return nil, err
	}

	netNodeManager, err := buildNetNodeManager(ctx, plgn, infraShim, serviceProcessor, collectorManager)
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
	region, instance, err := getRegionAndInstanceID(ctx)
	if err != nil {
		return nil, err
	}

	cloudManager, err := buildCloudManager(ctx, region, instance)
	if err != nil {
		return nil, err
	}

	infraShim, err := NewInfraShim(cloudManager)
	if err != nil {
		return nil, err
	}

	serviceProcessor := service.NewKubeProxyServiceProcessor(ctx)

	plgn, err := plugin.NewCalicoPlugin(ctx, serviceProcessor, infraShim)
	if err != nil {
		return nil, err
	}

	collectorManager, err := manager.NewSimplePodCollectorManager(ctx)
	if err != nil {
		return nil, err
	}

	netNodeManager, err := buildNetNodeManager(ctx, plgn, infraShim, serviceProcessor, collectorManager)
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

func NewTerwayNetwork() (network.Network, error) {
	return &terwayNetwork{}, nil
}

func (n *flannelNetwork) Diagnose(ctx *ctx.Context, src model.Endpoint, dst model.Endpoint) ([]model.Suspicion, *model.PacketPath, error) {
	return n.diagnostor.Diagnose(src, dst, model.Protocol(ctx.TaskConfig().Protocol))
}

func (n *calicoNetwork) Diagnose(ctx *ctx.Context, src model.Endpoint, dst model.Endpoint) ([]model.Suspicion, *model.PacketPath, error) {
	return n.diagnostor.Diagnose(src, dst, model.Protocol(ctx.TaskConfig().Protocol))
}

func (n *terwayNetwork) Diagnose(_ *ctx.Context, _ model.Endpoint, _ model.Endpoint) ([]model.Suspicion, *model.PacketPath, error) {
	// todo: implement me
	panic("implement me!")
}
