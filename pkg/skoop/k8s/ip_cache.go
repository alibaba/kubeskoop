package k8s

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/alibaba/kubeskoop/pkg/skoop/utils"
	"github.com/samber/lo"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"k8s.io/klog/v2"

	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type IPCache struct {
	k8sCli       *kubernetes.Clientset
	podCache     map[string]*v1.Pod
	nodeCache    map[string]*v1.Node
	serviceCache map[string]*v1.Service
	cacheOnce    *sync.Once
}

func NewIPCache(k8sCli *kubernetes.Clientset) *IPCache {
	return &IPCache{
		podCache:     map[string]*v1.Pod{},
		nodeCache:    map[string]*v1.Node{},
		serviceCache: map[string]*v1.Service{},
		k8sCli:       k8sCli,
		cacheOnce:    &sync.Once{},
	}
}

func (c *IPCache) ClearCache() {
	c.podCache = map[string]*v1.Pod{}
	c.nodeCache = map[string]*v1.Node{}
	c.serviceCache = map[string]*v1.Service{}
	c.cacheOnce = &sync.Once{}
}

func (c *IPCache) GetPodFromIP(ip string) (*v1.Pod, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return nil, err
	}

	return c.podCache[ip], nil
}

func (c *IPCache) GetPodFromName(namespace, name string) (*v1.Pod, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return nil, err
	}

	for _, pod := range c.podCache {
		if pod.Namespace == namespace && pod.Name == name {
			return pod, nil
		}
	}

	return nil, nil
}

func (c *IPCache) GetNodeFromIP(ip string) (*v1.Node, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return nil, err
	}

	return c.nodeCache[ip], nil
}

func (c *IPCache) GetNodeFromName(name string) (*v1.Node, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return nil, err
	}

	for _, node := range c.nodeCache {
		if node.Name == name {
			return node, nil
		}
	}

	return nil, nil
}

func (c *IPCache) GetServiceFromIP(ip string) (*v1.Service, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return nil, err
	}

	return c.serviceCache[ip], err
}

func (c *IPCache) GetServiceFromNodePort(nodePort uint16, protocol model.Protocol) (*v1.Service, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return nil, err
	}

	for _, svc := range c.serviceCache {
		for _, port := range svc.Spec.Ports {
			if uint16(port.NodePort) == nodePort &&
				strings.EqualFold(string(port.Protocol), string(protocol)) {
				return svc, nil
			}
		}
	}

	return nil, nil
}

func (c *IPCache) GetIPType(ip string) (model.EndpointType, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return "", err
	}

	if _, ok := c.podCache[ip]; ok {
		return model.EndpointTypePod, nil
	}

	if _, ok := c.nodeCache[ip]; ok {
		return model.EndpointTypeNode, nil
	}

	if svc, ok := c.serviceCache[ip]; ok {
		if svc.Spec.Type == v1.ServiceTypeLoadBalancer && utils.ContainsLoadBalancerIP(svc, ip) {
			return model.EndpointTypeLoadbalancer, nil
		}
		return model.EndpointTypeService, nil
	}

	return model.EndpointTypeExternal, nil
}

func (c *IPCache) BuildClusterIPCache() error {
	var innerErr error
	c.cacheOnce.Do(
		func() {
			var err error
			defer func() {
				if err != nil {
					innerErr = err
				}
			}()
			err = c.buildPodCache()
			if err != nil {
				return
			}
			err = c.buildNodeCache()
			if err != nil {
				return
			}
			err = c.buildServiceCache()
			if err != nil {
				return
			}
		})
	if innerErr != nil {
		c.cacheOnce = &sync.Once{}
		return fmt.Errorf("error build cluster ip cache: %v", innerErr)
	}
	return nil
}

func (c *IPCache) GetNodes() ([]*v1.Node, error) {
	err := c.BuildClusterIPCache()
	if err != nil {
		return nil, err
	}

	return lo.Values(c.nodeCache), nil
}

func (c *IPCache) addPodIPCache(ipaddr string, pod *v1.Pod) error {
	if priv, exist := c.podCache[ipaddr]; exist {
		klog.Warningf("Pod %s/%s address %s conflict with %s/%s, ignoring.", pod.Namespace, pod.Name, ipaddr, priv.Namespace, priv.Name)
		return nil
	}
	c.podCache[ipaddr] = pod
	return nil
}

func (c *IPCache) addNodeIPCache(ipaddr string, node *v1.Node) error {
	if priv, exist := c.nodeCache[ipaddr]; exist {
		klog.Warningf("Node %s address %s conflict with %s, ignoring.", node.Name, ipaddr, priv.Name)
		return nil
	}
	c.nodeCache[ipaddr] = node
	return nil
}
func (c *IPCache) addServiceIPCache(ipaddr string, svc *v1.Service) error {
	if priv, exist := c.serviceCache[ipaddr]; exist {
		klog.Warningf("Service %s/%s address %s conflict with %s/%s, ignoring.", svc.Namespace, svc.Name, ipaddr, priv.Namespace, priv.Name)
		return nil
	}
	c.serviceCache[ipaddr] = svc
	return nil
}

func (c *IPCache) buildPodCache() error {
	podList, err := c.k8sCli.CoreV1().Pods("").List(context.Background(), meta_v1.ListOptions{})
	if err != nil {
		return err
	}
	for _, pod := range podList.Items {
		if pod.Spec.HostNetwork || pod.Status.Phase != v1.PodRunning {
			klog.V(4).Infof("Pod %s/%s skipped, hostNetwork: %t, phase: %s, address: %v",
				pod.Namespace, pod.Name, pod.Spec.HostNetwork, pod.Status.Phase, pod.Status.PodIP)
			continue
		}
		copiedPod := pod.DeepCopy()
		for _, ip := range pod.Status.PodIPs {
			err = c.addPodIPCache(ip.IP, copiedPod)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *IPCache) buildServiceCache() error {
	svcList, err := c.k8sCli.CoreV1().Services("").List(context.Background(), meta_v1.ListOptions{})
	if err != nil {
		return err
	}
	for _, svc := range svcList.Items {
		copiedSvc := svc.DeepCopy()
		for _, clusterIP := range svc.Spec.ClusterIPs {
			// skip headless service
			if clusterIP == "None" {
				continue
			}

			err = c.addServiceIPCache(clusterIP, copiedSvc)
			if err != nil {
				return err
			}
		}
		for _, externalIP := range svc.Spec.ExternalIPs {
			err = c.addServiceIPCache(externalIP, copiedSvc)
			if err != nil {
				return err
			}
		}

		for _, lbIngress := range svc.Status.LoadBalancer.Ingress {
			err = c.addServiceIPCache(lbIngress.IP, copiedSvc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *IPCache) buildNodeCache() error {
	nodeList, err := c.k8sCli.CoreV1().Nodes().List(context.Background(), meta_v1.ListOptions{})
	if err != nil {
		return err
	}
	for _, node := range nodeList.Items {
		copiedNode := node.DeepCopy()
		for _, address := range node.Status.Addresses {
			err = c.addNodeIPCache(address.Address, copiedNode)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
