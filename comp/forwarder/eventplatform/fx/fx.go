package fx

import (
	"go.uber.org/fx"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params eventplatform.Params) fxutil.Module {
	return fxutil.Component(fx.Provide(eventplatformimpl.NewComponent), fx.Supply(params))
}
