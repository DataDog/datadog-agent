// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package agent

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Enabled determines whether the process agent is enabled based on the configuration.
// The process-agent component on linux can be run in the core agent or as a standalone process-agent
// depending on the configuration.
// It will run as a standalone Process-agent if 'run_in_core_agent' is not enabled or the connections/NPM check is
// enabled.
// If 'run_in_core_agent' flag is enabled and the connections/NPM check is not enabled, the process-agent will run in
// the core agent.
func Enabled(config config.Component, checkComponents []types.CheckComponent, log logComponent.Component) bool {
	runInCoreAgent := config.GetBool("process_config.run_in_core_agent.enabled")

	var npmEnabled bool
	for _, check := range checkComponents {
		if check.Object().Name() == checks.ConnectionsCheckName && check.Object().IsEnabled() {
			npmEnabled = true
			break
		}
	}

	switch flavor.GetFlavor() {
	case flavor.ProcessAgent:
		if npmEnabled {
			if runInCoreAgent {
				log.Info("Network Performance Monitoring is not supported in the core agent. " +
					"The process-agent will be enabled as a standalone agent")
			}
			return true
		}

		if runInCoreAgent {
			log.Info("The process checks will run in the core agent")
		}

		return !runInCoreAgent
	case flavor.DefaultAgent:
		if npmEnabled && runInCoreAgent {
			log.Info("Network Performance Monitoring is not supported in the core agent. " +
				"The process-agent will be enabled as a standalone agent to collect network performance metrics.")
		}
		return runInCoreAgent
	default:
		return false
	}
}
