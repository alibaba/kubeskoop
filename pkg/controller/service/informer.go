package service

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type ControllerInformer struct {
	k8sInformerFactory informers.SharedInformerFactory
	podInformer        v1.PodInformer
}

func (c *ControllerInformer) podListWithInformer(_ context.Context) ([]*Pod, error) {
	pods, err := c.podInformer.Lister().Pods("").List(labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list pods failed: %v", err)
	}
	return lo.Map[*corev1.Pod, *Pod](pods, func(pod *corev1.Pod, idx int) *Pod {
		return &Pod{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Nodename:  pod.Spec.NodeName,
			Labels:    pod.Labels,
		}
	}), nil
}

func (c *controller) InitInformer() {
	if c.k8sClient == nil {
		return
	}

	c.k8sInformerFactory = informers.NewSharedInformerFactory(c.k8sClient, time.Minute*1)
	c.podInformer = c.k8sInformerFactory.Core().V1().Pods()
	_ = c.podInformer.Informer().GetIndexer().AddIndexers(cache.Indexers{
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
}

func (c *controller) RunInformer(stop <-chan struct{}) {
	if c.k8sInformerFactory == nil {
		return
	}

	c.k8sInformerFactory.Start(stop)
	log.Infof("start informer cache sync.")
	if !cache.WaitForCacheSync(stop, c.podInformer.Informer().HasSynced) {
		log.Errorf("failed to sync pod info")
	}
	log.Infof("informer cache sync finish.")
}
