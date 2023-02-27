package nodemanager

import (
	"fmt"
	"sync"

	"github.com/alibaba/kubeskoop/pkg/skoop/collector"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/plugin"

	"k8s.io/client-go/kubernetes"
)

type NetNodeManager interface {
	GetNetNodeFromID(nodeType model.NetNodeType, id string) (model.NetNodeAction, error)
}

type defaultNetNodeManager struct {
	parent           NetNodeManager
	client           *kubernetes.Clientset
	ipCache          *k8s.IPCache
	plugin           plugin.Plugin
	collectorManager collector.Manager
	cache            sync.Map
}

func NewNetNodeManager(ctx *ctx.Context, networkPlugin plugin.Plugin, collectorManager collector.Manager) (NetNodeManager, error) {
	return &defaultNetNodeManager{
		client:           ctx.KubernetesClient(),
		ipCache:          ctx.ClusterConfig().IPCache,
		plugin:           networkPlugin,
		collectorManager: collectorManager,
	}, nil
}

func (m *defaultNetNodeManager) GetNetNodeFromID(nodeType model.NetNodeType, id string) (model.NetNodeAction, error) {
	key := m.cacheKey(nodeType, id)
	if node, ok := m.cache.Load(key); ok {
		return node.(model.NetNodeAction), nil
	}

	switch nodeType {
	case model.NetNodeTypePod:
		k8sPod, err := m.ipCache.GetPodFromIP(id)
		if err != nil {
			return nil, err
		}

		if k8sPod == nil {
			return nil, fmt.Errorf("k8s pod not found from ip %s", id)
		}

		podInfo, err := m.collectorManager.CollectPod(k8sPod.Namespace, k8sPod.Name)
		if err != nil {
			return nil, fmt.Errorf("error run collector for pod: %v", err)
		}

		return m.plugin.CreatePod(podInfo)
	case model.NetNodeTypeNode:
		nodeInfo, err := m.collectorManager.CollectNode(id)
		if err != nil {
			return nil, fmt.Errorf("error run collector for node: %v", err)
		}

		return m.plugin.CreateNode(nodeInfo)
	default:
		if m.parent != nil {
			return m.parent.GetNetNodeFromID(nodeType, id)
		}

		return &model.GenericNetNode{
			NetNode: &model.NetNode{
				Type:    model.NetNodeTypeGeneric,
				ID:      id,
				Actions: map[*model.Link]*model.Action{},
			},
		}, nil

	}
}

func (m *defaultNetNodeManager) cacheKey(typ model.NetNodeType, id string) string {
	return fmt.Sprintf("%s---%s", typ, id)
}

var _ NetNodeManager = &defaultNetNodeManager{}
