// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build orchestrator

// Package orchestratorimpl implements the orchestrator forwarder component.
package orchestratorimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newOrchestratorForwarder),
		fx.Supply(params))
}

// newOrchestratorForwarder returns an orchestratorForwarder
// if the feature is activated on the cluster-agent/cluster-check runner, nil otherwise
func newOrchestratorForwarder(log log.Component, config config.Component, secrets secrets.Component, tagger tagger.Component, lc fx.Lifecycle, params Params) orchestrator.Component {
	if params.useNoopOrchestratorForwarder {
		return createComponent(defaultforwarder.NoopForwarder{})
	}
	if params.useOrchestratorForwarder {
		isOrchestratorEnv := env.IsKubernetes() || env.IsECS() || env.IsECSFargate() || env.IsECSManagedInstances()
		orchestratorExplorerEnabled := config.GetBool(orchestratorconfig.OrchestratorNSKey("enabled"))
		if !orchestratorExplorerEnabled || !isOrchestratorEnv {
			forwarder := option.None[defaultforwarder.Forwarder]()
			return &forwarder
		}
		globalTags, err := tagger.GlobalTags(types.LowCardinality)
		if err != nil {
			log.Debugf("Error getting global tags for orchestrator config: %s", err)
		}
		orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig(globalTags)
		if err := orchestratorCfg.Load(); err != nil {
			log.Errorf("Error loading the orchestrator config: %s", err)
		}
		keysPerDomain := apicfg.KeysPerDomains(orchestratorCfg.OrchestratorEndpoints)
		resolver, err := resolver.NewSingleDomainResolvers(keysPerDomain)
		if err != nil {
			log.Errorf("Error creating domain resolver: %s", err)
		}
		orchestratorForwarderOpts := defaultforwarder.NewOptionsWithResolvers(config, log, resolver)
		orchestratorForwarderOpts.DisableAPIKeyChecking = true
		orchestratorForwarderOpts.Secrets = secrets

		forwarder := defaultforwarder.NewDefaultForwarder(config, log, orchestratorForwarderOpts)
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				_ = forwarder.Start()
				return nil
			}, OnStop: func(context.Context) error {
				forwarder.Stop()
				return nil
			}})

		return createComponent(forwarder)
	}

	forwarder := option.None[defaultforwarder.Forwarder]()
	return &forwarder
}

func createComponent(forwarder defaultforwarder.Forwarder) orchestrator.Component {
	o := option.New(forwarder)
	return &o
}
