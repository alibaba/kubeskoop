package provider

import (
	"fmt"

	ctx "github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/network"
)

const (
	providerNameGeneric = "generic"
	providerNameAliyun  = "aliyun"
)

type Provider interface {
	CreateNetwork(ctx *ctx.Context) (network.Network, error)
}

var providers = map[string]Provider{
	providerNameGeneric: genericProvider{},
	providerNameAliyun:  aliyunProvider{},
}

func GetProvider(name string) (Provider, error) {
	provider, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("service provider %q not found", name)
	}

	return provider, nil
}
