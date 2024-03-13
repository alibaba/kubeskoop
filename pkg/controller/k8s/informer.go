package k8s

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"time"
)

var sharedInformerFactory informers.SharedInformerFactory
var PodInformer v1.PodInformer
var NodeInformer v1.NodeInformer

func InitInformer(k8sClient kubernetes.Interface) error {
	if k8sClient == nil {
		return fmt.Errorf("invalid argument")
	}

	sharedInformerFactory = informers.NewSharedInformerFactory(k8sClient, time.Minute*1)
	PodInformer = sharedInformerFactory.Core().V1().Pods()
	NodeInformer = sharedInformerFactory.Core().V1().Nodes()

	_ = PodInformer.Informer().GetIndexer().AddIndexers(cache.Indexers{
		"nodeName": func(obj interface{}) ([]string, error) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return []string{}, nil
			}

			if pod.Status.Phase == corev1.PodRunning {
				return []string{}, nil
			}
			return []string{pod.Spec.NodeName}, nil
		},
	})

	return nil
}

func StartInformer(stop <-chan struct{}) {
	if sharedInformerFactory == nil {
		panic("informer not init")
	}

	sharedInformerFactory.Start(stop)
	log.Infof("start informer cache sync.")
	if !cache.WaitForCacheSync(stop, PodInformer.Informer().HasSynced) {
		log.Errorf("failed to sync pod info")
	}
	log.Infof("informer cache sync finish.")
}
