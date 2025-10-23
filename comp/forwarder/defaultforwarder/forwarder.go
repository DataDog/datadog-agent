// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
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

func newForwarder(dep dependencies) (provides, error) {
	options, err := createOptions(dep.Params, dep.Config, dep.Log)
	if err != nil {
		return provides{}, err
	}

	return NewForwarder(dep.Config, dep.Log, dep.Lc, true, options), nil
}

func createOptions(params Params, config config.Component, log log.Component) (*Options, error) {
	var options *Options
	keysPerDomain, err := utils.GetMultipleEndpoints(config)
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
		return nil, fmt.Errorf("Misconfiguration of agent endpoints: %s", err)
	}

	if !params.withResolver {
		options, err = NewOptions(config, log, keysPerDomain)
		if err != nil {
			log.Error("Error creating forwarder options: ", err)
			return nil, fmt.Errorf("Error creating forwarder options: %s", err)
		}
	} else {
		r, err := resolver.NewSingleDomainResolvers(keysPerDomain)
		if err != nil {
			log.Error("Error creating resolver: ", err)
			return nil, fmt.Errorf("Error creating resolver: %s", err)
		}
		options = NewOptionsWithResolvers(config, log, r)
	}
	// Override the DisableAPIKeyChecking only if WithFeatures was called
	if disableAPIKeyChecking, ok := params.disableAPIKeyCheckingOverride.Get(); ok {
		options.DisableAPIKeyChecking = disableAPIKeyChecking
	}
	options.SetEnabledFeatures(params.features)

	log.Infof("starting forwarder with %d endpoints", len(options.DomainResolvers))
	for _, resolver := range options.DomainResolvers {
		scrubbedKeys := []string{}
		for _, k := range resolver.GetAPIKeys() {
			scrubbedKeys = append(scrubbedKeys, scrubber.HideKeyExceptLastFiveChars(k))
		}
		log.Infof("domain '%s' has %d keys: %s", resolver.GetBaseDomain(), len(scrubbedKeys), strings.Join(scrubbedKeys, ", "))
	}
	return options, nil
}

// NewForwarder returns a new forwarder component.
//
//nolint:revive
func NewForwarder(config config.Component, log log.Component, lc fx.Lifecycle, ignoreLifeCycleError bool, options *Options) provides {
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
	options, _ := NewOptions(config, log, nil)
	return provides{
		Comp: NewDefaultForwarder(config, log, options),
	}
}
