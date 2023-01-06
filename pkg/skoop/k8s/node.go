package k8s

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"
)

type PodNetInfo struct {
	ContainerID  string `json:"container_id"`
	PodName      string `json:"pod_name"`
	PodNamespace string `json:"pod_namespace"`
	PodUID       string `json:"pod_uid"`
	PID          uint32 `json:"pid"`
	Netns        string `json:"netns"`
	HostNetwork  bool   `json:"host_network"`
	NetworkMode  string `json:"network_mode"`
}

type NodeNetworkStackDump struct {
	Pods  []PodNetInfo         `json:"pods"`
	Netns []netstack.NetNSInfo `json:"netns"`
}

type NodeMeta struct {
	NodeName string
}

type NodeInfo struct {
	netstack.NetNS
	SubNetNSInfo []netstack.NetNSInfo
	NodeMeta
}

type PodMeta struct {
	Namespace   string
	PodName     string
	NodeName    string
	HostNetwork bool
}

type Pod struct {
	model.NetNode
	netstack.NetNS
	PodMeta
}
