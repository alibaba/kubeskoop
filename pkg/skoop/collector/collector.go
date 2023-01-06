package collector

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
)

type Collector interface {
	DumpNodeInfos() (*k8s.NodeNetworkStackDump, error)
}

type Manager interface {
	CollectNode(nodename string) (*k8s.NodeInfo, error)
	CollectPod(namespace, name string) (*k8s.Pod, error)
}
