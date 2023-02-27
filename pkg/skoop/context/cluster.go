package context

import (
	"errors"
	"flag"
	"fmt"
	"net"

	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
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
	ClusterCIDRString string
	ClusterCIDR       *net.IPNet
	NetworkPlugin     string
	ProxyMode         string
	IPCache           *k8s.IPCache
}

func (cc *ClusterConfig) BindFlags(fs *pflag.FlagSet) {
	fs.StringVar(&cc.KubeConfigPath, "kube-config", "~/.kube/config", "Cluster kubeconfig file.")
	fs.StringVar(&cc.CloudProvider, "cloud-provider", "generic", "Cloud provider of cluster.")
	fs.StringVar(&cc.NetworkPlugin, "network-plugin", "", "Network plugin used in cluster. If not set, will try to auto detect it.")
	fs.StringVar(&cc.ProxyMode, "proxy-mode", "", "Proxy mode for kube-proxy. If not set, will try to detect it automatically.")
	fs.StringVar(&cc.ClusterCIDRString, "cluster-cidr", "", "Cluster pod CIDR. If not set, will try to detect it automatically.")
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
	c.ClusterConfig().IPCache = k8s.NewIPCache(clientSet)

	if c.ClusterConfig().NetworkPlugin == "" {
		plugin, err := utils.DetectNetworkPlugin(clientSet)
		if err != nil {
			return err
		}
		if plugin == "" {
			return errors.New("cannot auto detect network plugin, you can use \"--network-plugin\" to specify it")
		}
		c.ClusterConfig().NetworkPlugin = plugin
		klog.V(3).Infof("Detected network plugin %q.", plugin)
	} else {
		klog.V(3).Infof("Use provided network plugin %q.", c.ClusterConfig().NetworkPlugin)
	}

	if c.ClusterConfig().ProxyMode == "" {
		mode, err := utils.DetectKubeProxyMode(clientSet)
		if err != nil {
			return fmt.Errorf("cannoot auto detect kube-proxy mode: %v, you can use \"--proxy-mode\" to specify it", err)
		}
		if mode == "" {
			return fmt.Errorf("cannoot auto detect kube-proxy mode, you can use \"--proxy-mode\" to specify it")
		}

		c.ClusterConfig().ProxyMode = mode
		klog.V(3).Infof("Detected kube-proxy mode %q", mode)
	} else {
		klog.V(3).Infof("Use provided kube-proxy mode %q", c.ClusterConfig().ProxyMode)
	}

	if c.ClusterConfig().ClusterCIDRString == "" {
		clusterCIDR, err := utils.DetectClusterCIDR(clientSet)
		if err != nil {
			return fmt.Errorf("cannoot auto detect cluster cidr: %v, you can use \"--cluster-cidr\" to specify it", err)
		}
		if clusterCIDR == "" {
			return fmt.Errorf("cannoot auto detect clutser cidr, you can use \"--cluster-cidr\" to specify it")
		}

		_, cidr, err := net.ParseCIDR(clusterCIDR)
		if err != nil {
			return fmt.Errorf("cannot parse cluster cidr: %v", err)
		}
		c.ClusterConfig().ClusterCIDR = cidr
		klog.V(3).Infof("Detected cluster cidr %q", cidr)
	} else {
		_, cidr, err := net.ParseCIDR(c.ClusterConfig().ClusterCIDRString)
		if err != nil {
			return fmt.Errorf("cannot parse cluster cidr: %v", err)
		}
		c.ClusterConfig().ClusterCIDR = cidr
		klog.V(3).Infof("Use provided cluster cidr %q", c.ClusterConfig().ClusterCIDRString)
	}

	return nil
}
