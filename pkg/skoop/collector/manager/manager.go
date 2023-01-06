package manager

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	k8s2 "github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	netstack2 "github.com/alibaba/kubeskoop/pkg/skoop/netstack"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
)

const (
	defaultCollectorNamespace = "kubeskoop"

	defaultWaitInterval = 5
	defaultWaitTimeout  = 120

	hostnsKey = "/proc/1/ns/net"
)

type SimplePodCollectorManagerOptions struct {
	Image              string
	CollectorNamespace string
	WaitInterval       time.Duration
	WaitTimeout        time.Duration
}

type simplePodCollectorManager struct {
	image        string
	namespace    string
	client       *kubernetes.Clientset
	restConfig   *rest.Config
	ipCache      *k8s2.IPCache
	cache        map[string]*k8s2.NodeNetworkStackDump
	nodeCache    map[string]*k8s2.NodeInfo
	podCache     map[string]*k8s2.Pod
	waitInterval time.Duration
	waitTimeout  time.Duration
}

func NewSimplePodCollectorManager(ctx *ctx.Context) (collector.Manager, error) {
	if Config.SimplePodCollectorConfig.Image == "" {
		return nil, fmt.Errorf("image must be provided")
	}

	if Config.SimplePodCollectorConfig.CollectorNamespace == "" {
		Config.SimplePodCollectorConfig.CollectorNamespace = defaultCollectorNamespace
	}

	if Config.SimplePodCollectorConfig.WaitInterval == 0 {
		Config.SimplePodCollectorConfig.WaitInterval = defaultWaitInterval * time.Second
	}

	if Config.SimplePodCollectorConfig.WaitTimeout == 0 {
		Config.SimplePodCollectorConfig.WaitTimeout = defaultWaitTimeout * time.Second
	}

	return &simplePodCollectorManager{
		image:        Config.SimplePodCollectorConfig.Image,
		namespace:    Config.SimplePodCollectorConfig.CollectorNamespace,
		client:       ctx.KubernetesClient(),
		restConfig:   ctx.KubernetesRestClient(),
		ipCache:      ctx.ClusterConfig().IPCache,
		cache:        map[string]*k8s2.NodeNetworkStackDump{},
		nodeCache:    map[string]*k8s2.NodeInfo{},
		podCache:     map[string]*k8s2.Pod{},
		waitInterval: Config.SimplePodCollectorConfig.WaitInterval,
		waitTimeout:  Config.SimplePodCollectorConfig.WaitTimeout,
	}, nil
}

func (m *simplePodCollectorManager) CollectNode(nodename string) (*k8s2.NodeInfo, error) {
	if node, ok := m.nodeCache[nodename]; ok {
		return node, nil
	}

	err := m.buildCache(nodename)
	if err != nil {
		return nil, err
	}

	nodeInfo := m.nodeCache[nodename]
	if nodeInfo == nil {
		return nil, fmt.Errorf("cannot collect node %s", nodename)
	}

	m.nodeCache[nodename] = nodeInfo
	return nodeInfo, nil
}

func (m *simplePodCollectorManager) CollectPod(namespace, name string) (*k8s2.Pod, error) {
	podKey := fmt.Sprintf("%s/%s", namespace, name)
	if pod, ok := m.podCache[podKey]; ok {
		return pod, nil
	}

	pod, err := m.ipCache.GetPodFromName(namespace, name)
	if err != nil {
		return nil, err
	}

	if pod == nil {
		return nil, nil
	}

	err = m.buildCache(pod.Spec.NodeName)
	if err != nil {
		return nil, err
	}

	podInfo := m.podCache[podKey]
	if podInfo == nil {
		return nil, fmt.Errorf("cannot collect pod %s on node %s", podKey, pod.Spec.NodeName)
	}

	m.podCache[podKey] = podInfo

	return podInfo, nil
}

func (m *simplePodCollectorManager) buildCache(nodeName string) error {
	dump, err := m.collectNodeStackDump(nodeName)
	if err != nil {
		return err
	}

	netnsMap := map[string]netstack2.NetNSInfo{}

	for _, netns := range dump.Netns {
		netnsMap[netns.Netns] = netns
	}

	if _, ok := netnsMap[hostnsKey]; !ok {
		return fmt.Errorf("cannot get host netns info for node %s", nodeName)
	}

	hostNetNS := netnsMap[hostnsKey]
	nodeInfo := &k8s2.NodeInfo{
		SubNetNSInfo: dump.Netns,
		NetNS:        netstack2.NetNS{NetNSInfo: &hostNetNS},
		NodeMeta: k8s2.NodeMeta{
			NodeName: nodeName,
		},
	}

	nodeInfo.Router = netstack2.NewSimulateRouter(nodeInfo.NetNSInfo.RuleInfo, nodeInfo.NetNSInfo.RouteInfo, nodeInfo.NetNSInfo.Interfaces)
	nodeInfo.IPVS, err = netstack2.ParseIPVS(nodeInfo.NetNSInfo.IPVSInfo)
	nodeInfo.IPTables = netstack2.ParseIPTables(nodeInfo.NetNSInfo.IptablesInfo)
	if err != nil {
		return err
	}
	nodeInfo.IPSetManager, err = netstack2.ParseIPSet(nodeInfo.NetNSInfo.IpsetInfo)
	if err != nil {
		return err
	}
	nodeInfo.Interfaces = nodeInfo.NetNSInfo.Interfaces
	nodeInfo.Neighbour = netstack2.NewNeigh(nodeInfo.NetNSInfo.Interfaces)
	nodeInfo.Netfilter = netstack2.NewSimulateNetfilter(netstack2.SimulateNetfilterContext{
		IPTables: nodeInfo.IPTables,
		IPSet:    nodeInfo.IPSetManager,
		Router:   nodeInfo.Router,
		IPVS:     nodeInfo.IPVS,
	})

	m.nodeCache[nodeName] = nodeInfo

	for _, p := range dump.Pods {
		podInfo := &k8s2.Pod{
			PodMeta: k8s2.PodMeta{
				Namespace:   p.PodNamespace,
				PodName:     p.PodName,
				HostNetwork: p.NetworkMode == "host",
				NodeName:    nodeName,
			},
		}

		if podInfo.HostNetwork {
			podInfo.NetNSInfo = nodeInfo.NetNSInfo
		} else {
			podNetNS, ok := netnsMap[p.Netns]
			if !ok {
				return fmt.Errorf("cannot get pod netns info for pod %s/%s, node %s", p.PodNamespace, p.PodName, nodeName)
			}
			podInfo.NetNSInfo = &podNetNS
		}

		podInfo.IPVS, err = netstack2.ParseIPVS(podInfo.NetNSInfo.IPVSInfo)
		podInfo.IPTables = netstack2.ParseIPTables(podInfo.NetNSInfo.IptablesInfo)
		if err != nil {
			return err
		}
		podInfo.IPSetManager, err = netstack2.ParseIPSet(podInfo.NetNSInfo.IpsetInfo)
		if err != nil {
			return err
		}

		podInfo.Router = netstack2.NewSimulateRouter(podInfo.NetNSInfo.RuleInfo, podInfo.NetNSInfo.RouteInfo, podInfo.NetNSInfo.Interfaces)
		podInfo.Interfaces = podInfo.NetNSInfo.Interfaces
		podInfo.Neighbour = netstack2.NewNeigh(podInfo.NetNSInfo.Interfaces)
		podInfo.Netfilter = netstack2.NewSimulateNetfilter(netstack2.SimulateNetfilterContext{
			IPTables: podInfo.IPTables,
			IPSet:    podInfo.IPSetManager,
			Router:   podInfo.Router,
			IPVS:     podInfo.IPVS,
		})

		podKey := fmt.Sprintf("%s/%s", p.PodNamespace, p.PodName)

		m.podCache[podKey] = podInfo
	}

	return nil
}

func (m *simplePodCollectorManager) collectNodeStackDump(nodeName string) (*k8s2.NodeNetworkStackDump, error) {
	if dump, ok := m.cache[nodeName]; ok {
		return dump, nil
	}

	err := m.ensureNamespace()
	if err != nil {
		return nil, err
	}

	pod, err := m.createCollectorPod(nodeName)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := m.deleteCollectorPod(nodeName)
		if err != nil {
			klog.Errorf("failed delete collector pod: %s", err)
		}
	}()

	err = m.waitPodRunning(pod)
	if err != nil {
		return nil, err
	}

	data, err := m.readCollectorData(pod)
	if err != nil {
		return nil, err
	}

	m.cache[nodeName] = data
	return data, nil
}

func (m *simplePodCollectorManager) ensureNamespace() error {
	_, err := m.client.CoreV1().Namespaces().Get(context.TODO(), m.namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	if errors.IsNotFound(err) {
		ns := &v1.Namespace{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: m.namespace,
			},
		}
		_, err := m.client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		return nil
	}

	return err
}

func (m *simplePodCollectorManager) createCollectorPod(nodeName string) (*v1.Pod, error) {
	hostPathType := v1.HostPathDirectory
	podName := fmt.Sprintf("collector-%s", nodeName)
	err := m.ensurePodClean(podName)
	if err != nil {
		return nil, err
	}

	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.namespace,
			Name:      podName,
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name:            "collector",
					Image:           m.image,
					ImagePullPolicy: "Always",
					SecurityContext: &v1.SecurityContext{
						Privileged: pointer.Bool(true),
					},
					Command: []string{"/bin/pod-collector"},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "cri-dir",
							MountPath: "/var/run",
						},
						{
							Name:      "data",
							MountPath: "/data",
						},
						{
							Name:      "lib-modules",
							MountPath: "/lib/modules",
						},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:  "alive",
					Image: m.image,
					Command: []string{
						"/bin/sh",
						"-c",
						"while true;do sleep 100;done;",
					},
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "data",
							MountPath: "/data",
						},
					},
				},
			},
			NodeName:      nodeName,
			HostNetwork:   true,
			HostPID:       true,
			HostIPC:       true,
			RestartPolicy: "Never",
			Volumes: []v1.Volume{
				{
					Name: "cri-dir",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{
							Path: "/var/run",
							Type: &hostPathType,
						},
					},
				},
				{
					Name: "lib-modules",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{
							Path: "/lib/modules",
							Type: &hostPathType,
						},
					},
				},
				{
					Name: "data",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
		Status: v1.PodStatus{},
	}

	return m.client.CoreV1().Pods(m.namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
}

func (m *simplePodCollectorManager) deleteCollectorPod(nodeName string) error {
	podName := fmt.Sprintf("collector-%s", nodeName)
	err := m.client.CoreV1().Pods(m.namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func (m *simplePodCollectorManager) ensurePodClean(podName string) error {
	err := m.client.CoreV1().Pods(m.namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return err
	}

	err = wait.Poll(1*time.Second, 20*time.Second, func() (bool, error) {
		_, err := m.client.CoreV1().Pods(m.namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}

		if err != nil {
			return false, err
		}

		return false, nil
	})

	return err
}

func (m *simplePodCollectorManager) waitPodRunning(pod *v1.Pod) error {
	var lastError error
	_ = wait.PollImmediate(m.waitInterval, m.waitTimeout, func() (bool, error) {
		klog.V(2).Infof("Waiting pod %s/%s running", pod.Namespace, pod.Name)
		current, err := m.client.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
		if err != nil {
			lastError = err
			return false, nil
		}

		switch current.Status.Phase {
		case v1.PodRunning:
			lastError = nil
			return true, nil
		case v1.PodSucceeded, v1.PodFailed, v1.PodUnknown:
			lastError = fmt.Errorf("pod in unexpected status %s, log: %s", current.Status.Phase, m.getCollectorPodTailLog(pod))
			return false, lastError
		}

		return false, nil
	})

	if lastError != nil {
		return fmt.Errorf("wait pod running failed: %s", lastError)
	}

	return nil
}

func (m *simplePodCollectorManager) getCollectorPodTailLog(pod *v1.Pod) string {
	log, err := m.client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Namespace, &v1.PodLogOptions{
		Container: "collector",
		TailLines: pointer.Int64(10),
	}).Do(context.TODO()).Raw()

	if err != nil {
		return ""
	}

	return string(log)
}

func (m *simplePodCollectorManager) readCollectorData(pod *v1.Pod) (*k8s2.NodeNetworkStackDump, error) {
	req := m.client.CoreV1().RESTClient().Post().Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("exec").
		Param("container", "alive").
		VersionedParams(&v1.PodExecOptions{
			Stdout: true,
			Stderr: true,
			Command: []string{
				"sh",
				"-c",
				"cat /data/collector.json | base64",
			},
		}, scheme.ParameterCodec)

	outBuffer := &bytes.Buffer{}
	errBuffer := &bytes.Buffer{}

	exec, err := remotecommand.NewSPDYExecutor(m.restConfig, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: outBuffer,
		Stderr: errBuffer,
	})

	if err != nil {
		return nil, err
	}

	var dump = &k8s2.NodeNetworkStackDump{}
	output, err := base64.StdEncoding.DecodeString(outBuffer.String())
	if err != nil {
		return nil, fmt.Errorf("%s, stderr: %s", err, errBuffer.String())
	}

	err = json.Unmarshal(output, dump)
	if err != nil {
		return nil, fmt.Errorf("%s, stderr: %s", err, errBuffer.String())
	}

	return dump, nil
}
