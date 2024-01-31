// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agentimpl implements a component for the process agent.
package agentimpl

import (
	"context"

	"github.com/opentracing/opentracing-go/log"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	agent6DisabledMessage = `process-agent not enabled.
Set env var DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED=true or add
process_config:
  process_collection:
    enabled: true
to your datadog.yaml file.
Exiting.`
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newProcessAgent))
}

type processAgentParams struct {
	fx.In

	Lc     fx.Lifecycle
	Log    logComponent.Component
	Config config.Component
	Checks []types.CheckComponent `group:"check"`
	Runner runner.Component
}

type processAgent struct {
	Checks []checks.Check
	Log    logComponent.Component
	Runner runner.Component
}

func newProcessAgent(p processAgentParams) optional.Option[agent.Component] {
	if !agentEnabled(p.Config) {
		return optional.NewNoneOption[agent.Component]()
	}

	// Look to see if any checks are enabled, if not, return since the agent doesn't need to be enabled.
	if !checksEnabled(p.Checks) {
		p.Log.Info(agent6DisabledMessage)
		return optional.NewNoneOption[agent.Component]()
	}

	enabledChecks := make([]checks.Check, 0, len(p.Checks))
	for _, c := range p.Checks {
		check := c.Object()
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check)
		}
	}

	processAgentComponent := processAgent{
		Checks: enabledChecks,
		Log:    p.Log,
		Runner: p.Runner,
	}

	p.Lc.Append(fx.Hook{
		OnStart: processAgentComponent.Start,
		OnStop:  processAgentComponent.Stop,
	})

	return optional.NewOption[agent.Component](processAgentComponent)
}

func checksEnabled(checks []types.CheckComponent) bool {
	for _, check := range checks {
		if check.Object().IsEnabled() {
			return true
		}
	}
	return false
}

func (p processAgent) Start(ctx context.Context) error {
	p.Log.Info("process-agent starting")

	chks := make([]string, 0, len(p.Checks))
	for _, check := range p.Checks {
		chks = append(chks, check.Name())
	}
	p.Log.Info("process-agent checks", log.Object("checks", chks))

	return p.Runner.Run(ctx)
}

func (p processAgent) Stop(_ context.Context) error {
	p.Log.Info("process-agent stopping")

	return nil
}
