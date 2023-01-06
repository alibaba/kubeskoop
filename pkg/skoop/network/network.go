package network

import (
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	model2 "github.com/alibaba/kubeskoop/pkg/skoop/model"
	v1 "k8s.io/api/core/v1"
)

// InfraShim means ...
// todo: todo
type InfraShim interface {
	NodeToNode(src *v1.Node, oif string, dst *v1.Node, packet *model2.Packet) ([]model2.Suspicion, error)
	NodeToExternal(src *v1.Node, oif string, packet *model2.Packet) ([]model2.Suspicion, error)
}

type Network interface {
	Diagnose(ctx *ctx.Context, src model2.Endpoint, dst model2.Endpoint) ([]model2.Suspicion, *model2.PacketPath, error)
}
