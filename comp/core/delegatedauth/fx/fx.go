package fx

import (
	"go.uber.org/fx"

	delegatedauthimpl "github.com/DataDog/datadog-agent/comp/core/delegatedauth/impl"
)

// Module defines the fx options for this component
func Module() fx.Option {
	return fx.Module("delegatedauth",
		fx.Provide(delegatedauthimpl.NewDelegatedAuth),
	)
}
