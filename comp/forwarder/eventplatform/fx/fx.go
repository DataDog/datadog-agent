package fx

import (
	"go.uber.org/fx"

	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(fx.Provide(eventplatformimpl.NewComponent))
}
