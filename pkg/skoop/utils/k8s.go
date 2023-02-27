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

func DetectNetworkPlugin(k8sCli *kubernetes.Clientset) (networkMode string, err error) {
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
	return "", nil
}

var kubeProxyConfigmaps = []string{"kube-proxy", "kube-proxy-worker"}

func getKubeProxyConfigFromConfigMap(k8sCli *kubernetes.Clientset) (string, error) {
	var cm *v1.ConfigMap
	var err error
	for _, cmName := range kubeProxyConfigmaps {
		cm, err = k8sCli.CoreV1().ConfigMaps("kube-system").Get(context.Background(), cmName, metav1.GetOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return "", err
		}
		if err == nil {
			break
		}
	}
	if cm == nil {
		return "", fmt.Errorf("cannot find kube-proxy configmap")
	}
	return cm.Data["config.conf"], nil
}

func DetectKubeProxyMode(k8sCli *kubernetes.Clientset) (string, error) {
	conf, err := getKubeProxyConfigFromConfigMap(k8sCli)
	if err != nil {
		return "", err
	}

	if conf == "" {
		return "iptables", nil
	}

	cfg := struct {
		Mode string `yaml:"mode"`
	}{}
	err = yaml.Unmarshal([]byte(conf), &cfg)
	if err != nil {
		return "", err
	}
	if cfg.Mode == "" {
		return "iptables", nil
	}
	return cfg.Mode, nil
}

func DetectClusterCIDR(k8sCli *kubernetes.Clientset) (string, error) {
	conf, err := getKubeProxyConfigFromConfigMap(k8sCli)
	if err != nil {
		return "", err
	}

	cfg := struct {
		ClusterCIDR string `yaml:"clusterCIDR"`
	}{}
	err = yaml.Unmarshal([]byte(conf), &cfg)
	if err != nil {
		return "", err
	}
	return cfg.ClusterCIDR, nil
}
