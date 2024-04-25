package npschedulerimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	Lc          fx.Lifecycle
	EpForwarder eventplatform.Component
	Logger      log.Component
}

type provides struct {
	fx.Out

	Comp npscheduler.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newNpScheduler),
	)
}

func newNpScheduler(deps dependencies) provides {
	scheduler := newNpSchedulerImpl(deps.EpForwarder, deps.Logger)
	deps.Lc.Append(fx.Hook{
		// No need for OnStart hook since NpScheduler.Init() will be called by clients when needed.
		OnStop: func(context.Context) error {
			scheduler.Stop()
			return nil
		},
	})
	return provides{
		Comp: scheduler,
	}
}
