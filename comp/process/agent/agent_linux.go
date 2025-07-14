// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package agent

import (
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// List of check names for process checks

var (
	// enabled variable to ensure value returned by Enabled() persists when Enabled() is called multiple times
	enabled bool
	// Once module variable, exported for testing
	Once              sync.Once
	processCheckNames = []string{
		checks.ProcessCheckName,
		checks.ContainerCheckName,
		checks.DiscoveryCheckName,
	}
)

func enabledHelper(config config.Component, checkComponents []types.CheckComponent, l log.Component) bool {
	// never run the process component in the cluster worker
	if setup.IsCLCRunner(config) {
		return false
	}

	runInCoreAgent := config.GetBool("process_config.run_in_core_agent.enabled")

	var npmEnabled bool
	var processEnabled bool
	for _, check := range checkComponents {
		if check.Object().Name() == checks.ConnectionsCheckName && check.Object().IsEnabled() {
			npmEnabled = true
		}
		if slices.Contains(processCheckNames, check.Object().Name()) && check.Object().IsEnabled() {
			processEnabled = true
		}
	}

	switch flavor.GetFlavor() {
	case flavor.ProcessAgent:
		if npmEnabled {
			if runInCoreAgent {
				l.Info("Network Performance Monitoring is not supported in the core agent. " +
					"The process-agent will be enabled as a standalone agent")
			}
		}

		if runInCoreAgent {
			l.Info("The process checks will run in the core agent")
		} else if processEnabled {
			l.Info("Process/Container Collection in the Process Agent will be deprecated in a future release " +
				"and will instead be run in the Core Agent. " +
				"Set process_config.run_in_core_agent.enabled to true to switch now.")
		}

		return !runInCoreAgent || npmEnabled
	case flavor.DefaultAgent:
		if npmEnabled && runInCoreAgent {
			l.Info("Network Performance Monitoring is not supported in the core agent. " +
				"The process-agent will be enabled as a standalone agent to collect network performance metrics.")
		}
		return runInCoreAgent
	default:
		return false
	}
}

// Enabled determines whether the process agent is enabled based on the configuration.
// Enabled will only be run once, to prevent duplicate logging.
// The process-agent component on linux can be run in the core agent or as a standalone process-agent
// depending on the configuration.
// It will run as a standalone Process-agent if 'run_in_core_agent' is not enabled or the connections/NPM check is
// enabled.
// If 'run_in_core_agent' flag is enabled and the connections/NPM check is not enabled, the process-agent will run in
// the core agent.
func Enabled(config config.Component, checkComponents []types.CheckComponent, l log.Component) bool {
	Once.Do(func() {
		enabled = enabledHelper(config, checkComponents, l)
	})
	return enabled
}
