// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package agentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// agentEnabled determines whether the process agent is enabled based on the configuration.
// The process-agent component on linux can be run in the core agent or as a standalone process-agent
// depending on the configuration.
// It will run as a standalone Process-agent if 'run_in_core_agent' is not enabled or a the connections/NPM check is
// enabled.
// If 'run_in_core_agent' flag is enabled and the connections/NPM check is not enabled, the process-agent will run in
// the core agent.
func agentEnabled(p processAgentParams) bool {
	runInCoreAgent := p.Config.GetBool("process_config.run_in_core_agent.enabled")

	var npmEnabled bool
	for _, check := range p.Checks {
		if check.Object().Name() == checks.ConnectionsCheckName && check.Object().IsEnabled() {
			npmEnabled = true
			break
		}
	}

	switch flavor.GetFlavor() {
	case flavor.ProcessAgent:
		if npmEnabled {
			p.Log.Info("Network Performance Monitoring is enabled, " +
				"the container and connections check will run in the standalone process-agent.")
		}

		if runInCoreAgent {
			p.Log.Info("The process checks will run in the core agent")
		}

		return !runInCoreAgent || npmEnabled
	case flavor.DefaultAgent:
		if npmEnabled {
			p.Log.Info("Network Performance Monitoring is enabled, " +
				"the container and connections check will run in the standalone process-agent.")
		}

		return runInCoreAgent
	default:
		return false
	}
}
