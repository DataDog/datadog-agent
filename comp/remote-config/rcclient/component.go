package rcclient

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Component is the component type.
type Component interface {
	// TODO: (components) Start the remote config client to listen to AGENT_TASK configurations
	Listen() error
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newRemoteConfigClient),
)
