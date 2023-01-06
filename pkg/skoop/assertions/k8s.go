package assertions

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/skoop/model"

	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
)

type KubernetesAssertion struct {
	Assertion
}

func NewKubernetesAssertion(assertion Assertion) *KubernetesAssertion {
	return &KubernetesAssertion{Assertion: assertion}
}

func (a *KubernetesAssertion) AssertNode(node *v1.Node) {
	if readyStatus, ok := lo.Find(node.Status.Conditions, func(c v1.NodeCondition) bool { return c.Type == v1.NodeReady }); ok {
		AssertTrue(a, readyStatus.Status == v1.ConditionTrue, model.SuspicionLevelFatal,
			fmt.Sprintf("node ready status is %q, message: %s", readyStatus.Status, readyStatus.Message))
	}
}

func (a *KubernetesAssertion) AssertPod(pod *v1.Pod) {
	for _, c := range pod.Status.ContainerStatuses {
		AssertTrue(a, c.Ready, model.SuspicionLevelWarning, fmt.Sprintf("pod container %q is not ready.", c.Name))
	}
}
