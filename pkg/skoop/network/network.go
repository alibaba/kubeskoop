package network

import (
	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	v1 "k8s.io/api/core/v1"
)

// InfraShim means ...
// todo: todo
type InfraShim interface {
	NodeToNode(src *v1.Node, oif string, dst *v1.Node, packet *model.Packet) ([]model.Suspicion, error)
	NodeToExternal(src *v1.Node, oif string, packet *model.Packet) ([]model.Suspicion, error)
}

type Network interface {
	Diagnose(ctx *ctx.Context, src model.Endpoint, dst model.Endpoint) ([]model.Suspicion, *model.PacketPath, error)
}
