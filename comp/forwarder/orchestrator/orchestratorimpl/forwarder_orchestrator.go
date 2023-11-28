// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build orchestrator

// Package orchestratorimpl implements the orchestrator forwarder component.
package orchestratorimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newOrchestratorForwarder),
)

// newOrchestratorForwarder returns an orchestratorForwarder
// if the feature is activated on the cluster-agent/cluster-check runner, nil otherwise
func newOrchestratorForwarder(log log.Component, config config.Component, params Params) orchestrator.Component {
	if params.useNoopOrchestratorForwarder {
		return createComponent(defaultforwarder.NoopForwarder{})
	}
	if params.useOrchestratorForwarder {
		if !config.GetBool(orchestratorconfig.OrchestratorNSKey("enabled")) {
			forwarder := optional.NewNoneOption[defaultforwarder.Forwarder]()
			return &forwarder
		}
		orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig()
		if err := orchestratorCfg.Load(); err != nil {
			log.Errorf("Error loading the orchestrator config: %s", err)
		}
		keysPerDomain := apicfg.KeysPerDomains(orchestratorCfg.OrchestratorEndpoints)
		orchestratorForwarderOpts := defaultforwarder.NewOptionsWithResolvers(config, log, resolver.NewSingleDomainResolvers(keysPerDomain))
		orchestratorForwarderOpts.DisableAPIKeyChecking = true

		return createComponent(defaultforwarder.NewDefaultForwarder(config, log, orchestratorForwarderOpts))
	}

	forwarder := optional.NewNoneOption[defaultforwarder.Forwarder]()
	return &forwarder
}

func createComponent(forwarder defaultforwarder.Forwarder) orchestrator.Component {
	o := optional.NewOption(forwarder)
	return &o
}
