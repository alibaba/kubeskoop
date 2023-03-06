package provider

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

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
		supoortedProviders := lo.MapToSlice(providers, func(k string, _ Provider) string { return k })
		return nil, fmt.Errorf("service provider %q not found, supported providers: %s",
			name, strings.Join(supoortedProviders, ","))
	}

	return provider, nil
}
