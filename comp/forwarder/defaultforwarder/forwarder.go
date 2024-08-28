// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
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
	options := creationOptions(dep.Params, dep.Config, dep.Log)
	return NewForwarder(dep.Config, dep.Log, dep.Lc, true, options, dep.Params.UseNoopForwarder)
}

func creationOptions(params Params, config config.Component, log log.Component) *Options {
	var options *Options
	if !params.withResolver {
		options = NewOptions(config, log, getMultipleEndpoints(config, log))
	} else {
		keysPerDomain := getMultipleEndpoints(config, log)
		options = NewOptionsWithResolvers(config, log, resolver.NewSingleDomainResolvers(keysPerDomain))
	}
	if disableAPIKeyChecking, ok := params.disableAPIKeyCheckingOverride.Get(); ok {
		options.DisableAPIKeyChecking = disableAPIKeyChecking
	}
	for _, feature := range params.features {
		options.EnabledFeatures = SetFeature(options.EnabledFeatures, feature)
	}
	return options
}

func getMultipleEndpoints(config config.Component, log log.Component) map[string][]string {
	// Inject the config to make sure we can call GetMultipleEndpoints.
	keysPerDomain, err := utils.GetMultipleEndpoints(config)
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	return keysPerDomain
}

// NewForwarder returns a new forwarder component.
//
//nolint:revive
func NewForwarder(config config.Component, log log.Component, lc fx.Lifecycle, ignoreLifeCycleError bool, options *Options, useNoopForwarder bool) provides {
	if useNoopForwarder {
		return provides{
			Comp: NoopForwarder{},
		}
	}
	forwarder := NewDefaultForwarder(config, log, options)

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
