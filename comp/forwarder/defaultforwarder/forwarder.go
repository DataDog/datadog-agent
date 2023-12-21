// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	Config config.Component
	Log    log.Component
	Params Params
}

type provides struct {
	fx.Out

	Comp           Component
	StatusProvider status.InformationProvider
}

func newForwarder(dep dependencies) provides {
	return NewForwarder(dep.Config, dep.Log, dep.Params)
}

//nolint:revive // TODO(ASC) Fix revive linter
func NewForwarder(config config.Component, log log.Component, params Params) provides {
	if params.UseNoopForwarder {
		return provides{
			Comp: NoopForwarder{},
			StatusProvider: status.NewInformationProvider(statusProvider{
				config: config,
			}),
		}
	}
	return provides{
		Comp: NewDefaultForwarder(config, log, params.Options),
		StatusProvider: status.NewInformationProvider(statusProvider{
			config: config,
		}),
	}
}

func newMockForwarder(config config.Component, log log.Component) Component {
	return NewDefaultForwarder(config, log, NewOptions(config, log, nil))
}
