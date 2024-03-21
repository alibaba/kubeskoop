package k8s

import (
	"os"

	"github.com/alibaba/kubeskoop/pkg/skoop/utils"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var Client *kubernetes.Clientset

type Config struct {
	KubeConfig string `yaml:"kubeConfig"`
}

func InitKubernetesClient(config *Config) error {
	var (
		restConfig *rest.Config
		err        error
	)

	if config.KubeConfig != "" {
		log.Infof("load kubeconfig from %s", config.KubeConfig)
		restConfig, _, err = utils.NewConfig(config.KubeConfig)
	} else if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		log.Infof("load incluster kubeconfig")
		restConfig, err = rest.InClusterConfig()
	} else {
		log.Infof("try load kubeconfig from ~/.kube/config")
		restConfig, _, err = utils.NewConfig("~/.kube/config")
	}
	if err != nil {
		return err
	}

	Client, err = kubernetes.NewForConfig(restConfig)
	return err
}
