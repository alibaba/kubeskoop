package skoop

import (
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/skoop/assertions"
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/nodemanager"
	"github.com/alibaba/kubeskoop/pkg/skoop/plugin"
	"github.com/alibaba/kubeskoop/pkg/skoop/utils"
	log "github.com/sirupsen/logrus"
)

type Diagnostor interface {
	Diagnose(src, dst model.Endpoint, protocol model.Protocol) ([]model.Suspicion, *model.PacketPath, error)
}

type defaultDiagnostor struct {
	ctx                  *ctx.Context
	netNodeManager       nodemanager.NetNodeManager
	networkPolicyHandler plugin.NetworkPolicyHandler
}

func NewDefaultDiagnostor(ctx *ctx.Context, netNodeManager nodemanager.NetNodeManager, networkPolicy plugin.NetworkPolicyHandler) (Diagnostor, error) {
	return &defaultDiagnostor{
		ctx:                  ctx,
		netNodeManager:       netNodeManager,
		networkPolicyHandler: networkPolicy,
	}, nil
}

func toNodeType(typ model.EndpointType) model.NetNodeType {
	switch typ {
	case model.EndpointTypePod:
		return model.NetNodeTypePod
	case model.EndpointTypeNode:
		return model.NetNodeTypeNode
	default:
		return model.NetNodeTypeGeneric
	}
}

func (d *defaultDiagnostor) createNode(ep model.Endpoint) (model.NetNodeAction, error) {
	if ep.Type == model.EndpointTypeNode {
		node, err := d.ctx.ClusterConfig().IPCache.GetNodeFromIP(ep.IP)
		if err != nil {
			return nil, err
		}
		return d.netNodeManager.GetNetNodeFromID(model.NetNodeTypeNode, node.Name)
	}
	return d.netNodeManager.GetNetNodeFromID(toNodeType(ep.Type), ep.IP)
}

func (d *defaultDiagnostor) diagnoseNetworkPolicy(src, dst model.Endpoint, protocol model.Protocol) []model.Suspicion {
	if d.networkPolicyHandler == nil {
		return nil
	}

	ret, err := d.networkPolicyHandler.CheckNetworkPolicy(src, dst, protocol)
	if err != nil {
		log.Errorf("networkpolicy diangose err: %v", err)
		return nil
	}
	return ret
}

func (d *defaultDiagnostor) Diagnose(src, dst model.Endpoint, protocol model.Protocol) ([]model.Suspicion, *model.PacketPath, error) {
	globalSuspicion := d.diagnoseNetworkPolicy(src, dst, protocol)

	srcNode, err := d.createNode(src)
	if err != nil {
		return globalSuspicion, nil, err
	}

	danglingTransmissions := utils.NewQueue[model.Transmission]()
	transmissions, err := srcNode.Send(dst, protocol)
	if err != nil {
		if e, ok := err.(*assertions.CannotBuildTransmissionError); ok {
			//log
			return globalSuspicion, model.NewPacketPath(e.SrcNode), nil
		}
		return globalSuspicion, nil, err
	}

	if len(transmissions) == 0 {
		return globalSuspicion, nil, fmt.Errorf("unexpected zero size transmission output")
	}

	srcNetNode := transmissions[0].Link.Source

	graph := model.NewPacketPath(srcNetNode)

	danglingTransmissions.Enqueue(transmissions...)

	for !danglingTransmissions.Empty() {
		trans := danglingTransmissions.Pop()
		nextHopNode, err := d.netNodeManager.GetNetNodeFromID(trans.NextHop.Type, trans.NextHop.ID)
		if err != nil {
			return globalSuspicion, graph, err
		}

		generated, err := nextHopNode.Receive(trans.Link)
		if err != nil {
			log.Errorf("node [%s]%s failed do receive action, %v", trans.NextHop.Type, trans.NextHop.ID, err)
			return globalSuspicion, graph, nil
		}

		danglingTransmissions.Enqueue(generated...)
	}

	return globalSuspicion, graph, nil
}
