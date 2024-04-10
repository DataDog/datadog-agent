// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package agentimpl implements a component for the process agent.
package agentimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	statusComponent "github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	submitterComp "github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	agentDisabledMessage = `process-agent not enabled.
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

	Lc        fx.Lifecycle
	Log       logComponent.Component
	Config    config.Component
	Checks    []types.CheckComponent `group:"check"`
	Runner    runner.Component
	Submitter submitterComp.Component
}

type processAgent struct {
	enabled bool
	Checks  []checks.Check
	Log     logComponent.Component
}

type provides struct {
	fx.Out

	Comp           agent.Component
	StatusProvider statusComponent.InformationProvider
}

func newProcessAgent(p processAgentParams) provides {
	if !agent.Enabled(p.Config, p.Checks, p.Log) {
		return provides{
			Comp: processAgent{
				enabled: false,
			},
		}
	}

	enabledChecks := make([]checks.Check, 0, len(p.Checks))
	for _, c := range fxutil.GetAndFilterGroup(p.Checks) {
		check := c.Object()
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check)
		}
	}

	// Look to see if any checks are enabled, if not, return since the agent doesn't need to be enabled.
	if len(enabledChecks) == 0 {
		p.Log.Info(agentDisabledMessage)
		return provides{
			Comp: processAgent{
				enabled: false,
			},
		}
	}

	processAgentComponent := processAgent{
		enabled: true,
		Checks:  enabledChecks,
		Log:     p.Log,
	}

	if flavor.GetFlavor() == flavor.ProcessAgent {
		return provides{
			Comp: processAgentComponent,
		}
	}

	return provides{
		Comp:           processAgentComponent,
		StatusProvider: statusComponent.NewInformationProvider(agent.NewStatusProvider(p.Config)),
	}
}

// Enabled determines whether the process agent is enabled based on the configuration.
func (p processAgent) Enabled() bool {
	return p.enabled
}
