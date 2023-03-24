package apiserver

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/comp/core/log"
)

type apiserver struct {
}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Log log.Component
}

func newApiServer(deps dependencies) Component {
	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			err := api.StartServer()
			if err != nil {
				return err
			}

			return nil
		},
	})

	return &apiserver{}
}
