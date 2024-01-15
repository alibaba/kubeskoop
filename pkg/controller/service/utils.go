package service

import (
	"context"
	"fmt"
	"github.com/alibaba/kubeskoop/pkg/controller/rpc"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *controller) getPodInfo(ctx context.Context, podNamespace, podName string) (*rpc.PodInfo, *rpc.NodeInfo, string, error) {
	p, err := c.k8sClient.CoreV1().Pods(podNamespace).Get(ctx, podName, v1.GetOptions{})
	if err != nil {
		return nil, nil, "", fmt.Errorf("get pod %s/%s failed: %v", podNamespace, podName, err)
	}
	if p.Status.Phase != corev1.PodRunning {
		return nil, nil, "", fmt.Errorf("pod %s/%s is not running", podNamespace, podName)
	}
	pi := &rpc.PodInfo{
		Name:      podName,
		Namespace: podNamespace,
	}
	ni := &rpc.NodeInfo{
		Name: p.Spec.NodeName,
	}
	return pi, ni, p.Status.PodIP, nil
}

func (c *controller) getNodeInfo(ctx context.Context, nodeName string) (*rpc.NodeInfo, string, error) {
	p, err := c.k8sClient.CoreV1().Nodes().Get(ctx, nodeName, v1.GetOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("get node %s failed: %v", nodeName, err)
	}
	ni := &rpc.NodeInfo{
		Name: p.Name,
	}
	if len(p.Status.Addresses) < 1 {
		return nil, "", fmt.Errorf("node %s has no address", nodeName)
	}

	for _, addr := range p.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return ni, addr.Address, nil
		}
	}

	return ni, p.Status.Addresses[0].Address, nil
}

func (c *controller) getConfigMap(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
	cm, err := c.k8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get configmap %s/%s failed: %v", namespace, name, err)
	}
	return cm, nil
}

func (c *controller) updateConfigMap(ctx context.Context, namespace, name string, cm *corev1.ConfigMap) error {
	_, err := c.k8sClient.CoreV1().ConfigMaps(namespace).Update(ctx, cm, v1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update configmap %s/%s failed: %v", namespace, name, err)
	}
	return nil
}
