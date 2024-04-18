package collectorcontribFx

import (
	collectorcontrib "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def"
	collectorcontribImpl "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() collectorcontrib.Component {
			// TODO: (agent-shared-components) use fxutil.ProvideComponentConstruct once it is implemented
			// See the RFC "fx-decoupled components" for more details
			return collectorcontribImpl.NewComponent()
		}))
}
