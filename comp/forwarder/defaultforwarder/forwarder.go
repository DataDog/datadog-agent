// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
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

// NewForwarder returns a new forwarder component.
func NewForwarder(config config.Component, log log.Component, params Params) provides {
	if params.UseNoopForwarder {
		return provides{
			Comp:           NoopForwarder{},
			StatusProvider: status.NoopInformationProvider(),
		}
	}
	return provides{
		Comp:           NewDefaultForwarder(config, log, params.Options),
		StatusProvider: status.NewInformationProvider(statusProvider{config: config}),
	}
}

func newMockForwarder(config config.Component, log log.Component) provides {
	return provides{
		Comp:           NewDefaultForwarder(config, log, NewOptions(config, log, nil)),
		StatusProvider: status.NoopInformationProvider(),
	}
}
