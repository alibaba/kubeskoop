package framework

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/ui"

	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/pointer"
)

const (
	PodImage     = "nicolaka/netshoot"
	E2ENamespace = "default"
)

type Framework struct {
	client            *kubernetes.Clientset
	restConfig        *rest.Config
	executor          skoopExecutor
	extraDiagnoseArgs []string
	nodes             []v1.Node
	nodeExecutor      map[string]*v1.Pod

	spec        *TestSpec
	result      *DiagnoseResult
	commandInfo commandInfo

	kubeConfig     string
	cloudProvider  string
	collectorImage string
}

func NewFramework(c *kubernetes.Clientset, rest *rest.Config, skoopPath string,
	cloudProvider, kubeConfig, collectorImage string, extraDiagnoseArgs []string) (*Framework, error) {
	executor := newDirectExecutor(skoopPath)
	f := &Framework{
		executor:          executor,
		client:            c,
		nodeExecutor:      make(map[string]*v1.Pod),
		extraDiagnoseArgs: extraDiagnoseArgs,
	}

	nodes, err := f.listNodes()
	if err != nil {
		return nil, err
	}
	f.nodes = nodes
	f.restConfig = rest
	f.cloudProvider = cloudProvider
	f.kubeConfig = kubeConfig
	f.collectorImage = collectorImage

	return f, nil
}

func (f *Framework) SetTestSpec(spec *TestSpec) {
	f.spec = spec
	f.result = nil
}

func (f *Framework) Prepare() error {
	if f.spec == nil {
		return fmt.Errorf("node spec not set")
	}

	err := f.setNodeCommandInfo()
	if err != nil {
		return fmt.Errorf("set node command info failed: %s", err)
	}

	err = f.prepareService()
	if err != nil {
		return fmt.Errorf("prepare service failed: %s", err)
	}

	// prepare nodes
	for _, n := range f.spec.NodeSpecs {
		err = f.prepareNode(n)
		if err != nil {
			return fmt.Errorf("prepare node %s failed: %s", n.ID, err)
		}
	}

	err = f.waitE2EPodsReady()
	if err != nil {
		return fmt.Errorf("wait e2e pods failed: %s", err)
	}

	err = f.mapPodsToSpec(f.spec.NodeSpecs)
	if err != nil {
		return fmt.Errorf("map pods to spec failed: %s", err)
	}

	err = f.setPodCommandInfo()
	if err != nil {
		return fmt.Errorf("set pod command info failed: %s", err)
	}

	// apply node commands
	for _, n := range f.spec.NodeSpecs {
		err = f.applyNodeCommands(n.node, n.Commands)
		if err != nil {
			return fmt.Errorf("apply command to node %s failed: %s", n.ID, err)
		}
	}

	return nil
}

func (f *Framework) Recover() error {
	if f.spec == nil {
		return fmt.Errorf("node spec not set")
	}

	for _, n := range f.spec.NodeSpecs {
		err := f.applyNodeCommands(n.node, n.RecoverCommands)
		if err != nil {
			return fmt.Errorf("apply command to node %s failed: %s", n.ID, err)
		}
	}

	err := f.deleteE2EPods()
	if err != nil {
		return fmt.Errorf("delete e2e pods failed: %s", err)
	}

	err = f.deleteE2EService()
	if err != nil {
		return fmt.Errorf("delete e2e service failed: %s", err)
	}

	err = f.waitE2EPodsClean()
	if err != nil {
		return fmt.Errorf("wait e2e pods clean failed: %s", err)
	}

	return nil
}

func (f *Framework) deleteE2EPods() error {
	pods, err := f.listE2EPods()
	if err != nil {
		return err
	}

	for _, p := range pods {
		err = f.client.CoreV1().Pods(p.Namespace).Delete(context.TODO(), p.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *Framework) deleteE2EService() error {
	svcs, err := f.listE2EService()
	if err != nil {
		return err
	}

	for _, svc := range svcs {
		err = f.client.CoreV1().Services(svc.Namespace).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *Framework) RunDiagnose() error {
	spec := f.spec.DiagnoseSpec

	src := f.getIP(spec.From)
	if src == "" {
		return fmt.Errorf("cannot get ip for id %q", spec.From)
	}
	dst := f.getIP(spec.To)
	if dst == "" {
		return fmt.Errorf("cannot get ip for id %q", spec.To)
	}

	args := DiagnoseArgs{
		Src:            src,
		Dst:            dst,
		Port:           spec.Port,
		Protocol:       string(spec.Protocol),
		CloudProvider:  f.cloudProvider,
		KubeConfig:     f.kubeConfig,
		CollectorImage: f.collectorImage,
		ExtraArgs:      f.extraDiagnoseArgs,
	}

	result, err := f.executor.Diagnose(args)
	if err != nil {
		return err
	}

	f.result = &result
	return nil
}

func (f *Framework) getIP(id string) string {
	for _, n := range f.spec.NodeSpecs {
		if n.ID == id {
			return n.node.Status.Addresses[0].Address
		}
		for _, p := range n.PodSpecs {
			if p.ID == id {
				return p.pod.Status.PodIP
			}
		}
	}

	for _, s := range f.spec.ServiceSpecs {
		if s.ID == id {
			return s.service.Spec.ClusterIP
		}
	}

	if net.ParseIP(id) != nil {
		return id
	}

	return ""
}

func (f *Framework) AssignNodes() error {
	var workerNodes []*v1.Node
	for i := range f.nodes {
		if _, ok := f.nodes[i].Labels["node-role.kubernetes.io/control-plane"]; !ok {
			workerNodes = append(workerNodes, &f.nodes[i])
		}
	}

	lo.Shuffle(workerNodes)
	spec := f.spec.NodeSpecs

	if len(workerNodes) < len(spec) {
		return fmt.Errorf("node in cluster is not enough, expected: %d, actual %d", len(spec), len(f.nodes))
	}

	for i := range spec {
		spec[i].node = workerNodes[i]
	}

	return nil
}

func (f *Framework) prepareNode(spec *NodeSpec) error {
	if spec.node == nil {
		return fmt.Errorf("no node assigned to this spec")
	}

	err := f.setNodeExecutor()
	if err != nil {
		return err
	}

	for _, p := range spec.PodSpecs {
		_, err := f.createE2EPod(p, spec.node.Name)
		if err != nil {
			return err
		}
	}

	if spec.Listen != 0 {
		_, err := f.createHostNetworkPod(spec)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *Framework) applyNodeCommands(node *v1.Node, commands []string) error {
	if len(commands) == 0 {
		return nil
	}

	executorPod, ok := f.nodeExecutor[node.Name]
	if !ok {
		return fmt.Errorf("cannot find executor pod on node %s", node.Name)
	}

	var renderedCommands []string
	for _, cmd := range commands {
		rendered, err := f.renderCommand(cmd)
		if err != nil {
			return err
		}
		renderedCommands = append(renderedCommands, rendered)
	}

	err := f.execCommand(executorPod, renderedCommands)
	if err != nil {
		return fmt.Errorf("command on %s failed, commands: %s, error: %s", node.Name, renderedCommands, err)
	}

	return nil
}

func (f *Framework) createE2EPod(spec *PodSpec, nodeName string) (*v1.Pod, error) {
	podTemplate, err := f.createPodTemplateFromPodSpec(spec, nodeName)
	if err != nil {
		return nil, err
	}
	return f.client.CoreV1().Pods(podTemplate.Namespace).Create(context.TODO(), podTemplate, metav1.CreateOptions{})
}

func (f *Framework) createHostNetworkPod(spec *NodeSpec) (*v1.Pod, error) {
	podTemplate, err := createPodTemplateFromNodeSpec(spec)
	if err != nil {
		return nil, err
	}
	podTemplate.Spec.HostNetwork = true
	return f.client.CoreV1().Pods(podTemplate.Namespace).Create(context.TODO(), podTemplate, metav1.CreateOptions{})
}

func (f *Framework) listNodes() ([]v1.Node, error) {
	nodes, err := f.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return nodes.Items, nil
}

func (f *Framework) setNodeExecutor() error {
	pods, err := f.client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{
				"role": "executor",
			},
			MatchExpressions: nil,
		}),
	})
	if err != nil {
		return err
	}

	for i := range pods.Items {
		f.nodeExecutor[pods.Items[i].Spec.NodeName] = &pods.Items[i]
	}

	return nil
}

func (f *Framework) execCommand(pod *v1.Pod, commands []string) error {
	execCommand := []string{
		"/bin/sh",
		"-c",
		strings.Join(commands, "\n"),
	}
	req := f.client.CoreV1().RESTClient().Post().Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: "alive",
			Stdout:    true,
			Stderr:    true,
			Command:   execCommand,
		}, scheme.ParameterCodec)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exec, err := remotecommand.NewSPDYExecutor(f.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
	})
	outputString := stderr.String()
	if err != nil {
		return fmt.Errorf("error occurred while executing command on pod %s/%s, err: %s, stderr: %s",
			pod.Namespace, pod.Name, err, outputString)
	}

	if outputString != "" {
		return fmt.Errorf("error occurred while executing command on pod %s/%s, stderr: %s",
			pod.Namespace, pod.Name, outputString)
	}

	return nil
}

func (f *Framework) listE2EPods() ([]v1.Pod, error) {
	pods, err := f.client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{
				"role": "e2e-pod",
			},
		}),
	})

	if err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func (f *Framework) mapPodsToSpec(spec []*NodeSpec) error {
	pods, err := f.listE2EPods()
	if err != nil {
		return err
	}

	m := map[string]*v1.Pod{}
	for i := range pods {
		m[pods[i].Labels["e2e-id"]] = &pods[i]
	}

	for _, n := range spec {
		n.skoopID = getSkoopNodeID(n.node)
		for _, p := range n.PodSpecs {
			pod := m[p.ID]
			if pod == nil {
				return fmt.Errorf("actual pod not found for pod id %s", p.ID)
			}
			p.pod = pod
			p.skoopID = getSkoopPodID(pod)
		}
	}

	return nil
}

func (f *Framework) waitE2EPodsReady() error {
	var lastError error
	_ = wait.Poll(time.Second*1, time.Minute*2, func() (bool, error) {
		pods, err := f.listE2EPods()
		if err != nil {
			lastError = err
			return false, nil
		}

		for _, p := range pods {
			if p.Status.Phase != v1.PodRunning {
				lastError = fmt.Errorf("pod %s is %s, not Running", p.Name, p.Status.Phase)
				return false, nil
			}
		}

		lastError = nil
		return true, nil
	})

	if lastError != nil {
		return fmt.Errorf("wait pods running failed: %s", lastError)
	}

	return nil
}

func (f *Framework) waitE2EPodsClean() error {
	var lastError error
	_ = wait.PollImmediate(time.Second*1, time.Minute*2, func() (bool, error) {
		pods, err := f.listE2EPods()
		if err != nil {
			lastError = err
			return false, nil
		}

		if len(pods) != 0 {
			podNames := lo.Map(pods, func(p v1.Pod, _ int) string { return p.Name })
			lastError = fmt.Errorf("pods are not cleaned, remaining: %v", podNames)
		}

		lastError = nil
		return true, nil
	})

	if lastError != nil {
		return fmt.Errorf("wait pods running failed: %s", lastError)
	}

	return nil
}

func (f *Framework) Assert() error {
	if f.result == nil {
		return fmt.Errorf("result is nil")
	}

	assertion := &f.spec.Assertion

	if f.result.Succeed != assertion.Succeed {
		return fmt.Errorf("expected success status %t, but actual %t, error output: %s",
			assertion.Succeed, f.result.Succeed, f.result.Error)
	}

	if !f.result.Succeed {
		return nil
	}

	// make id to skoopID
	idMap := map[string]string{}
	for _, n := range f.spec.NodeSpecs {
		idMap[n.ID] = n.skoopID
		for _, p := range n.PodSpecs {
			idMap[p.ID] = p.skoopID
		}
	}

	var skoopNodeIDs []string
	for _, id := range f.spec.Assertion.Nodes {
		skoopID, ok := idMap[id]
		if !ok && net.ParseIP(id) != nil {
			idMap[id] = id // add ip to idMap
			skoopID = id
			ok = true
		}

		if !ok {
			return fmt.Errorf("cannot find id %s in skoop IDs", id)
		}
		skoopNodeIDs = append(skoopNodeIDs, skoopID)
	}

	// compare Nodes
	actualSkoopIDs := lo.Map(f.result.Summary.Nodes, func(n ui.DiagnoseSummaryNode, _ int) string { return n.ID })
	d1, d2 := lo.Difference(skoopNodeIDs, actualSkoopIDs)
	if len(d1) != 0 || len(d2) != 0 {
		return fmt.Errorf("node differences found, assertion: %v, actual: %v", skoopNodeIDs, actualSkoopIDs)
	}

	// assert suspicions
	err := f.assertSuspicions(idMap)
	if err != nil {
		return err
	}

	// asset actions
	err = f.assertActions(idMap)
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) assertSuspicions(idMap map[string]string) error {
	foundSuspicion := false
	actualSuspicions := map[string][]ui.DiagnoseSummarySuspicion{}
	for _, n := range f.result.Summary.Nodes {
		actualSuspicions[n.ID] = n.Suspicions
		if len(n.Suspicions) != 0 {
			foundSuspicion = true
		}
	}

	if f.spec.Assertion.NoSuspicion && foundSuspicion {
		suspicions := lo.SliceToMap(f.result.Summary.Nodes, func(n ui.DiagnoseSummaryNode) (string, []ui.DiagnoseSummarySuspicion) {
			return n.ID, n.Suspicions
		})
		return fmt.Errorf("assert no suspicion, but suspicions found: %+v", suspicions)
	}

	for _, s := range f.spec.Assertion.Suspicions {
		skoopID, ok := idMap[s.On]
		if !ok {
			return fmt.Errorf("cannot find skoop id for id %s", s.On)
		}

		suspicions, ok := actualSuspicions[skoopID]
		if !ok {
			return fmt.Errorf("cannot find actual node for skoop id %s", skoopID)
		}

		_, found := lo.Find(suspicions, func(actual ui.DiagnoseSummarySuspicion) bool {
			return actual.Level == s.Level && strings.Contains(actual.Message, s.Contains)
		})

		if !found {
			return fmt.Errorf("cannot find suspicion {on: %s, level: %s, contains: %s} on node %s, actual suspicions: %+v",
				s.On, s.Level, s.Contains, skoopID, suspicions)
		}
	}

	return nil
}

func (f *Framework) assertActions(idMap map[string]string) error {
	actualActions := map[string][]ui.DiagnoseSummaryNodeAction{}
	for _, n := range f.result.Summary.Nodes {
		actualActions[n.ID] = lo.MapToSlice(n.Actions,
			func(_ string, a ui.DiagnoseSummaryNodeAction) ui.DiagnoseSummaryNodeAction { return a })
	}

	for _, a := range f.spec.Assertion.Actions {
		skoopID, ok := idMap[a.On]
		if !ok {
			return fmt.Errorf("cannot find skoop id for id %s", a.On)
		}

		actions, ok := actualActions[skoopID]
		if !ok {
			return fmt.Errorf("cannot find actual node for skoop id %s", skoopID)
		}

		_, found := lo.Find(actions, func(actual ui.DiagnoseSummaryNodeAction) bool {
			return actual.Type == a.Type
		})

		if !found {
			return fmt.Errorf("cannot find action { on: %s, type: %s } on node %s, actual actions: %+v",
				a.On, a.Type, skoopID, actions)
		}
	}

	return nil
}

func (f *Framework) prepareService() error {
	for _, s := range f.spec.ServiceSpecs {
		svc, err := createServiceTemplate(s.ID,
			map[string]string{"role": "e2e-service", "e2e": "yes"},
			map[string]string{fmt.Sprintf("svc-%s", s.ID): "yes"},
			s.Port, s.TargetPort, s.TargetPortName, s.Protocol)

		if err != nil {
			return err
		}

		svc, err = f.client.CoreV1().Services(E2ENamespace).Create(context.TODO(), svc, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		s.service = svc

		// add ownerService to pods
		for _, n := range f.spec.NodeSpecs {
			for _, p := range n.PodSpecs {
				if slices.Contains(s.Endpoints, p.ID) {
					p.ownerService = append(p.ownerService, s.ID)
				}
			}
		}
	}

	return nil
}

func (f *Framework) listE2EService() ([]v1.Service, error) {
	svc, err := f.client.CoreV1().Services(E2ENamespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: map[string]string{"role": "e2e-service"},
		}),
	})

	if err != nil {
		return nil, err
	}
	return svc.Items, nil
}

// func (f *Framework) getPodFromID(id string) *v1.Pod {
// 	for _, n := range f.spec.NodeSpecs {
// 		for _, p := range n.PodSpecs {
// 			if p.ID == id {
// 				return p.pod
// 			}
// 		}
// 	}
// 	return nil
// }

func (f *Framework) createPodTemplateFromPodSpec(spec *PodSpec, nodeName string) (*v1.Pod, error) {
	aliveCommand := "sleep 30d"
	if spec.Listen != 0 {
		switch spec.ListenProtocol {
		default:
			aliveCommand = fmt.Sprintf("nc -l %d", spec.Listen)
		case model.UDP:
			aliveCommand = fmt.Sprintf("nc -ul %d", spec.Listen)
		}
	}

	var initCommand []string
	for _, cmd := range spec.Commands {
		rendered, err := f.renderCommand(cmd)
		if err != nil {
			return nil, err
		}
		initCommand = append(initCommand, rendered)
	}

	labels := map[string]string{
		"role":   "e2e-pod",
		"e2e":    "yes",
		"e2e-id": spec.ID,
	}

	for _, svc := range spec.ownerService {
		labels[fmt.Sprintf("svc-%s", svc)] = "yes"
	}

	return createPodTemplate(fmt.Sprintf("e2e-pod-%s", spec.ID), nodeName,
		labels, spec.Annotations, strings.Join(initCommand, "\n"), aliveCommand), nil
}

func (f *Framework) Node(id string) *v1.Node {
	for _, n := range f.spec.NodeSpecs {
		if n.ID == id {
			return n.node
		}
	}
	return nil
}

func (f *Framework) Pod(id string) *v1.Pod {
	for _, n := range f.spec.NodeSpecs {
		for _, p := range n.PodSpecs {
			if p.ID == id {
				return p.pod
			}
		}
	}
	return nil
}

func (f *Framework) Service(id string) *v1.Service {
	for _, s := range f.spec.ServiceSpecs {
		if s.ID == id {
			return s.service
		}
	}
	return nil
}

func (f *Framework) Client() *kubernetes.Clientset {
	return f.client
}

func (f *Framework) RestConfig() *rest.Config {
	return f.restConfig
}

func createPodTemplateFromNodeSpec(spec *NodeSpec) (*v1.Pod, error) {
	var aliveCommand string
	switch spec.ListenProtocol {
	default:
		aliveCommand = fmt.Sprintf("nc -l %d", spec.Listen)
	case model.UDP:
		aliveCommand = fmt.Sprintf("nc -ul %d", spec.Listen)
	}

	return createPodTemplate(fmt.Sprintf("e2e-pod-hostnetwork-%s", spec.ID), spec.node.Name,
		map[string]string{"role": "e2e-pod", "e2e": "yes", "e2e-id": spec.ID}, nil, "", aliveCommand), nil
}

func createPodTemplate(name, nodeName string, labels map[string]string, annotations map[string]string, initCommand string, aliveCommand string) *v1.Pod {
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   E2ENamespace,
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name:            "run-command",
					Image:           PodImage,
					ImagePullPolicy: "IfNotPresent",
					SecurityContext: &v1.SecurityContext{
						Privileged: pointer.Bool(true),
					},
					Command: []string{
						"/bin/sh",
						"-c",
						initCommand,
					},
				},
			},
			Containers: []v1.Container{
				{
					Name:  "alive",
					Image: PodImage,
					Command: []string{
						"/bin/sh",
						"-c",
						aliveCommand,
					},
					SecurityContext: &v1.SecurityContext{
						Privileged: pointer.Bool(true),
					},
				},
			},
			NodeName:                      nodeName,
			RestartPolicy:                 "Never",
			TerminationGracePeriodSeconds: pointer.Int64(0),
		},
		Status: v1.PodStatus{},
	}
}

func createServiceTemplate(name string, labels map[string]string, selector map[string]string, port uint16, targetPort uint16, targetPortName string, protocol model.Protocol) (*v1.Service, error) {
	v1Protocol := v1.ProtocolTCP
	if protocol == model.UDP {
		v1Protocol = v1.ProtocolUDP
	}

	target := intstr.FromInt(int(targetPort))
	if targetPortName != "" {
		target = intstr.FromString(targetPortName)
	}

	return &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: E2ENamespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:       "port",
					Protocol:   v1Protocol,
					Port:       int32(port),
					TargetPort: target,
				},
			},
			Selector: selector,
		},
	}, nil
}
