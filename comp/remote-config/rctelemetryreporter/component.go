package rctelemetryreporter

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: remote-config

// Component is the component type.
type Component interface {
	IncTimeout()
	IncRateLimit()
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewDdRcTelemetryReporter))
}
