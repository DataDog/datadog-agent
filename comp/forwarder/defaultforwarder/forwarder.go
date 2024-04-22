// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

type dependencies struct {
	fx.In
	Config config.Component
	Log    log.Component
	Lc     fx.Lifecycle
	Params Params
}

type provides struct {
	fx.Out

	Comp           Component
	StatusProvider status.InformationProvider
}

func newForwarder(dep dependencies) provides {
	return NewForwarder(dep.Config, dep.Log, dep.Lc, true, dep.Params)
}

// NewForwarder returns a new forwarder component.
//
//nolint:revive
func NewForwarder(config config.Component, log log.Component, lc fx.Lifecycle, ignoreLifeCycleError bool, params Params) provides {
	if params.UseNoopForwarder {
		return provides{
			Comp: NoopForwarder{},
		}
	}
	forwarder := NewDefaultForwarder(config, log, params.Options)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			err := forwarder.Start()
			if ignoreLifeCycleError {
				return nil
			}
			return err
		},
		OnStop: func(context.Context) error { forwarder.Stop(); return nil }})

	return provides{
		Comp:           forwarder,
		StatusProvider: status.NewInformationProvider(statusProvider{config: config}),
	}
}

func newMockForwarder(config config.Component, log log.Component) provides {
	return provides{
		Comp: NewDefaultForwarder(config, log, NewOptions(config, log, nil)),
	}
}
