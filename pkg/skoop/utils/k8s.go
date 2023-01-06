package utils

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewConfig returns a new Kubernetes configuration object
func NewConfig(kubeconfigPath string) (*rest.Config, *clientcmd.ClientConfig, error) {
	var err error
	var cc *rest.Config

	if kubeconfigPath == "" {
		return nil, nil, fmt.Errorf("kubeconfig path is invalid")
	}

	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{})

	cc, err = kubeconfig.ClientConfig()
	if err == nil {
		return cc, &kubeconfig, nil
	}

	kubeconfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{})

	cc, err = kubeconfig.ClientConfig()
	if err == nil {
		return cc, &kubeconfig, nil
	}

	return nil, nil, fmt.Errorf("Failed to load Kubernetes config: %s", err)
}

func Normalize(objType string, obj interface{}) string {
	type normalize interface {
		GetNamespace() string
		GetName() string
	}
	objMeta, ok := obj.(normalize)
	if ok {
		return fmt.Sprintf("%s/%s/%s", objType, objMeta.GetNamespace(), objMeta.GetName())
	}
	return ""
}

func ClusterNetworkConfig(k8sCli *kubernetes.Clientset) (ipvsmode, clusterCIDR, networkName string, err error) {
	ipvsmode, clusterCIDR, err = kubeproxyConfig(k8sCli)
	if err != nil {
		return "", "", "", err
	}
	networkName, err = networkMode(k8sCli)
	if err != nil {
		return "", "", "", err
	}
	return
}

func networkMode(k8sCli *kubernetes.Clientset) (networkMode string, err error) {
	dss, err := k8sCli.AppsV1().DaemonSets("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, ds := range dss.Items {
		switch ds.Name {
		case "kube-flannel-ds":
			return "flannel", nil
		case "calico-node":
			return "calico", nil
		case "terway-eniip":
			return "terway-eniip", nil
		}
	}
	return "generic", nil
}

var kubeproxyConfigMap = []string{"kube-proxy", "kube-proxy-worker"}

func kubeproxyConfig(k8sCli *kubernetes.Clientset) (proxyMode string, clusterCIDR string, err error) {
	var kubeproxyCM *v1.ConfigMap
	for _, cmName := range kubeproxyConfigMap {
		kubeproxyCM, err = k8sCli.CoreV1().ConfigMaps("kube-system").Get(context.Background(), cmName, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return "", "", err
		}
		if err == nil {
			break
		}
	}
	if kubeproxyCM == nil {
		return "", "", fmt.Errorf("cannot found kube-proxy configmap for ProxyMode & ClusterCIDR")
	}
	configYaml, ok := kubeproxyCM.Data["config.conf"]
	if !ok {
		proxyMode = "iptables"
	} else {
		kubeproxyCfg := struct {
			Mode        string `yaml:"mode"`
			ClusterCIDR string `yaml:"clusterCIDR"`
		}{}
		err = yaml.Unmarshal([]byte(configYaml), &kubeproxyCfg)
		if err != nil {
			return "", "", err
		}
		proxyMode = kubeproxyCfg.Mode
		clusterCIDR = kubeproxyCfg.ClusterCIDR
	}
	return proxyMode, clusterCIDR, nil
}
