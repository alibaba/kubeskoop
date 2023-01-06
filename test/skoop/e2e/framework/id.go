package framework

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

// FIXME: 统一各处处理ID的接口
func getSkoopPodID(pod *v1.Pod) string {
	// pod's ID is namespace/name
	return fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
}

func getSkoopNodeID(node *v1.Node) string {
	// node's ID is node name
	return node.Name
}
