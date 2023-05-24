package k8s

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/netstack"
)

type PodNetInfo struct {
	ContainerID  string `json:"id"`
	PodName      string `json:"n"`
	PodNamespace string `json:"ns"`
	PodUID       string `json:"u"`
	PID          uint32 `json:"pid"`
	Netns        string `json:"net"`
	HostNetwork  bool   `json:"hn"`
	NetworkMode  string `json:"nm"`
}

type NodeNetworkStackDump struct {
	Pods  []PodNetInfo         `json:"p"`
	Netns []netstack.NetNSInfo `json:"n"`
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
