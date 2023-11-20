// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build orchestrator

// Package forwarderimpl implements the orchestrator forwarder component.
package forwarderimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/orchestrator/forwarder"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(NewOrchestratorForwarder),
)

// NewOrchestratorForwarder returns an orchestratorForwarder
// if the feature is activated on the cluster-agent/cluster-check runner, nil otherwise
func NewOrchestratorForwarder(log log.Component, config config.Component, params Params) forwarder.Component {
	if params.UseNoopOrchestratorForwarder {
		return defaultforwarder.NoopForwarder{}
	}
	if params.UseOrchestratorForwarder {

		if !config.GetBool(orchestratorconfig.OrchestratorNSKey("enabled")) {
			return nil
		}
		orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig()
		if err := orchestratorCfg.Load(); err != nil {
			log.Errorf("Error loading the orchestrator config: %s", err)
		}
		keysPerDomain := apicfg.KeysPerDomains(orchestratorCfg.OrchestratorEndpoints)
		orchestratorForwarderOpts := defaultforwarder.NewOptionsWithResolvers(config, log, resolver.NewSingleDomainResolvers(keysPerDomain))
		orchestratorForwarderOpts.DisableAPIKeyChecking = true

		return defaultforwarder.NewDefaultForwarder(config, log, orchestratorForwarderOpts)
	}

	return nil // TODO: (Components): Use optional instead
}
