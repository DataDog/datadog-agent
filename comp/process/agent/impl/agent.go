// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package agentimpl implements a component for the process agent.
package agentimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	statusComponent "github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	agent "github.com/DataDog/datadog-agent/comp/process/agent/def"
	expvars "github.com/DataDog/datadog-agent/comp/process/expvars/impl"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/def"
	runner "github.com/DataDog/datadog-agent/comp/process/runner/def"
	submitterComp "github.com/DataDog/datadog-agent/comp/process/submitter/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
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

// NewComponent creates a new process agent component.
func NewComponent(deps dependencies) (Provides, error) {
	return newProcessAgent(deps)
}

type dependencies struct {
	compdef.In

	Lc             compdef.Lifecycle
	Log            log.Component
	Config         config.Component
	Checks         []types.CheckComponent `group:"check"`
	Runner         runner.Component
	Submitter      submitterComp.Component
	SysProbeConfig sysprobeconfig.Component
	HostInfo       hostinfo.Component
	Hostname       hostnameinterface.Component
}

type processAgent struct {
	enabled     bool
	Checks      []checks.Check
	Log         log.Component
	flarehelper *FlareHelper
}

type Provides struct {
	compdef.Out

	Comp           agent.Component
	StatusProvider statusComponent.InformationProvider
	FlareProvider  flaretypes.Provider
}

func newProcessAgent(deps dependencies) (Provides, error) {
	if !Enabled(deps.Config, deps.Checks, deps.Log) {
		return Provides{
			Comp: processAgent{
				enabled: false,
			},
		}, nil
	}

	enabledChecks := make([]checks.Check, 0, len(deps.Checks))
	for _, c := range deps.Checks {
		if c == nil {
			continue
		}
		check := c.Object()
		if check.IsEnabled() {
			enabledChecks = append(enabledChecks, check)
		}
	}

	// Look to see if any checks are enabled, if not, return since the agent doesn't need to be enabled.
	if len(enabledChecks) == 0 {
		deps.Log.Info(agentDisabledMessage)
		return Provides{
			Comp: processAgent{
				enabled: false,
			},
		}, nil
	}

	processAgentComponent := processAgent{
		enabled:     true,
		Checks:      enabledChecks,
		Log:         deps.Log,
		flarehelper: NewFlareHelper(enabledChecks),
	}

	if flavor.GetFlavor() != flavor.ProcessAgent {
		// We return a status provider when the component is used outside of the process agent
		// as the component status is unique from the typical agent status in this case.
		err := expvars.InitProcessStatus(deps.Config, deps.SysProbeConfig, deps.HostInfo, deps.Log)
		if err != nil {
			_ = deps.Log.Critical("Failed to initialize process status server:", err)
		}
		return Provides{
			Comp:           processAgentComponent,
			StatusProvider: statusComponent.NewInformationProvider(NewStatusProvider(deps.Config, deps.Hostname)),
			FlareProvider:  flaretypes.NewProvider(processAgentComponent.flarehelper.FillFlare),
		}, nil
	}

	return Provides{
		Comp: processAgentComponent,
	}, nil
}

// Enabled determines whether the process agent is enabled based on the configuration.
func (p processAgent) Enabled() bool {
	return p.enabled
}
