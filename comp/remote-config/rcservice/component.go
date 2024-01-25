package rcservice

import (
	"context"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: remote-config

// Component is the component type.
type Component interface {
	Start(ctx context.Context)
	Stop() error
	ClientGetConfigs(_ context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
	ConfigGetState() (*pbgo.GetStateConfigResponse, error)
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteConfigService))
}
