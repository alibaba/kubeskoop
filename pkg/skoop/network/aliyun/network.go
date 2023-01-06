package aliyun

import (
	"context"
	"fmt"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	"github.com/alibaba/kubeskoop/pkg/skoop/collector/manager"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	aliyun2 "github.com/alibaba/kubeskoop/pkg/skoop/infra/aliyun"
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
	"github.com/alibaba/kubeskoop/pkg/skoop/nodemanager"
	plugin2 "github.com/alibaba/kubeskoop/pkg/skoop/plugin"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"
	"github.com/alibaba/kubeskoop/pkg/skoop/skoop"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type flannelNetwork struct {
	plugin           plugin2.Plugin
	diagnostor       skoop.Diagnostor
	collectorManager collector.Manager
	netNodeManager   nodemanager.NetNodeManager
}

type calicoNetwork struct {
	plugin           plugin2.Plugin
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
		return "", "", fmt.Errorf("cannot find region or instance id in cluster")
	}

	return region, instance, nil
}

func buildCloudManager(ctx *ctx.Context, region, instanceOfCluster string) (*aliyun2.CloudManager, error) {
	options := &aliyun2.CloudManagerOptions{
		Region:            region,
		AccessKeyID:       aliyun2.Config.AccessKeyID,
		AccessKeySecret:   aliyun2.Config.AccessKeySecret,
		SecurityToken:     aliyun2.Config.SecurityToken,
		InstanceOfCluster: instanceOfCluster,
	}

	return aliyun2.NewCloudManager(options)
}

func getFlannelCNIMode(ctx *ctx.Context) (plugin2.FlannelBackendType, error) {
	cfg, err := ctx.KubernetesClient().CoreV1().
		ConfigMaps("kube-system").Get(context.TODO(), "kube-flannel-cfg", metav1.GetOptions{})
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

	cniMode, err := getFlannelCNIMode(ctx)
	if err != nil {
		return nil, err
	}

	serviceProcessor := service.NewKubeProxyServiceProcessor(ctx)
	options := &plugin2.FlannelPluginOptions{
		InfraShim:        infraShim,
		Bridge:           "cni0",
		Interface:        "eth0",
		PodMTU:           1500,
		IPMasq:           true,
		ClusterCIDR:      ctx.ClusterConfig().ClusterCIDR,
		CNIMode:          cniMode,
		ServiceProcessor: service.NewKubeProxyServiceProcessor(ctx),
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
	options := &plugin2.CalicoPluginOptions{
		InfraShim:        infraShim,
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

func NewTerwayNetwork() (network.Network, error) {
	return &terwayNetwork{}, nil
}

func (n *flannelNetwork) Diagnose(ctx *ctx.Context, src model2.Endpoint, dst model2.Endpoint) ([]model2.Suspicion, *model2.PacketPath, error) {
	return n.diagnostor.Diagnose(src, dst, model2.Protocol(ctx.TaskConfig().Protocol))
}

func (n *calicoNetwork) Diagnose(ctx *ctx.Context, src model2.Endpoint, dst model2.Endpoint) ([]model2.Suspicion, *model2.PacketPath, error) {
	return n.diagnostor.Diagnose(src, dst, model2.Protocol(ctx.TaskConfig().Protocol))
}

func (n *terwayNetwork) Diagnose(ctx *ctx.Context, src model2.Endpoint, dst model2.Endpoint) ([]model2.Suspicion, *model2.PacketPath, error) {
	// todo: implement me
	panic("implement me!")
}
