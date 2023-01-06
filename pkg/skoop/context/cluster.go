package context

import (
	"flag"
	"fmt"
	"net"

	kubernetes2 "github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"

	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	NetworkPluginFlannel = "flannel"
	NetworkPluginCalico  = "calico"
	NetworkPluginTerway  = "terway"
)

type ClusterConfig struct {
	KubeConfigPath    string
	CloudProvider     string
	ClusterCIDR       *net.IPNet
	NetworkPluginName string
	ProxyMode         string
	IPCache           *kubernetes2.IPCache
}

func (cc *ClusterConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cc.KubeConfigPath, "kube-config", "~/.kube/config", "Cluster kubeconfig file.")
	fs.StringVar(&cc.CloudProvider, "cloud-provider", "generic", "Cloud provider of cluster.")
	logFlag := flag.NewFlagSet("", flag.ExitOnError)
	klog.InitFlags(logFlag)
	fs.AddGoFlagSet(logFlag)
}

func (cc *ClusterConfig) Validate() error {
	return nil
}

func (c *Context) ClusterConfig() *ClusterConfig {
	clusterConfig, _ := c.Ctx.Load(clusterConfigKey)
	return clusterConfig.(*ClusterConfig)
}

func (c *Context) KubernetesClient() *kubernetes.Clientset {
	k8sCli, _ := c.Ctx.Load(kubernetesClientKey)
	return k8sCli.(*kubernetes.Clientset)
}

func (c *Context) KubernetesRestClient() *rest.Config {
	k8sRest, _ := c.Ctx.Load(kubernetesRestConfigKey)
	return k8sRest.(*rest.Config)
}

func (c *Context) BuildCluster() error {
	//TODO cluster的一些信息是不是应该放到provider中
	restConfig, _, err := utils.NewConfig(c.ClusterConfig().KubeConfigPath)
	if err != nil {
		return fmt.Errorf("error init kubernetes client from %v, error: %v", c.ClusterConfig().KubeConfigPath, err)
	}
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("error init kubernetes rest client from %v, error: %v", c.ClusterConfig().KubeConfigPath, err)
	}
	SkoopContext.Ctx.Store(kubernetesRestConfigKey, restConfig)
	SkoopContext.Ctx.Store(kubernetesClientKey, clientSet)
	c.ClusterConfig().IPCache = kubernetes2.NewIPCache(clientSet)

	proxyMode, clusterCIDR, networkName, err := utils.ClusterNetworkConfig(clientSet)
	if err != nil {
		return err
	}
	if clusterCIDR != "" {
		_, clusterCIDRNet, err := net.ParseCIDR(clusterCIDR)
		if err != nil {
			return err
		}
		c.ClusterConfig().ClusterCIDR = clusterCIDRNet
	}
	c.ClusterConfig().NetworkPluginName = networkName
	c.ClusterConfig().ProxyMode = proxyMode
	return nil
}
