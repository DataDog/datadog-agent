// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package agentimpl implements a component for the process agent.
package agentimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	statusComponent "github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	expvars "github.com/DataDog/datadog-agent/comp/process/expvars/expvarsimpl"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
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

type dependencies struct {
	fx.In

	Lc             fx.Lifecycle
	Log            logComponent.Component
	Config         config.Component
	Checks         []types.CheckComponent `group:"check"`
	Runner         runner.Component
	Submitter      submitterComp.Component
	SysProbeConfig sysprobeconfig.Component
	HostInfo       hostinfo.Component
	Telemetry      telemetry.Component
}

type processAgent struct {
	enabled     bool
	Checks      []checks.Check
	Log         logComponent.Component
	flarehelper *agent.FlareHelper
}

type provides struct {
	fx.Out

	Comp           agent.Component
	StatusProvider statusComponent.InformationProvider
	FlareProvider  flaretypes.Provider
}

func newProcessAgent(deps dependencies) provides {
	if !agent.Enabled(deps.Config, deps.Checks, deps.Log) {
		return provides{
			Comp: processAgent{
				enabled: false,
			},
		}
	}

	enabledChecks := make([]checks.Check, 0, len(deps.Checks))
	for _, c := range fxutil.GetAndFilterGroup(deps.Checks) {
		check := c.Object()
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check)
		}
	}

	// Look to see if any checks are enabled, if not, return since the agent doesn't need to be enabled.
	if len(enabledChecks) == 0 {
		deps.Log.Info(agentDisabledMessage)
		return provides{
			Comp: processAgent{
				enabled: false,
			},
		}
	}

	processAgentComponent := processAgent{
		enabled:     true,
		Checks:      enabledChecks,
		Log:         deps.Log,
		flarehelper: agent.NewFlareHelper(enabledChecks),
	}

	if flavor.GetFlavor() != flavor.ProcessAgent {
		// We return a status provider when the component is used outside of the process agent
		// as the component status is unique from the typical agent status in this case.
		err := expvars.InitProcessStatus(deps.Config, deps.SysProbeConfig, deps.HostInfo, deps.Log, deps.Telemetry)
		if err != nil {
			_ = deps.Log.Critical("Failed to initialize process status server:", err)
		}
		return provides{
			Comp:           processAgentComponent,
			StatusProvider: statusComponent.NewInformationProvider(agent.NewStatusProvider(deps.Config)),
			FlareProvider:  flaretypes.NewProvider(processAgentComponent.flarehelper.FillFlare),
		}
	}

	return provides{Comp: processAgentComponent}
}

// Enabled determines whether the process agent is enabled based on the configuration.
func (p processAgent) Enabled() bool {
	return p.enabled
}
