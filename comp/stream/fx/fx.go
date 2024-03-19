package streamfx

import (
	streamimpl "github.com/DataDog/datadog-agent/comp/stream/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(streamimpl.NewStream))
}
