// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build aix

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

var (
	enabled bool
	Once    sync.Once

	processCheckNames = []string{
		checks.ProcessCheckName,
		checks.ContainerCheckName,
		checks.DiscoveryCheckName,
	}
)

func enabledHelper(config config.Component, checkComponents []types.CheckComponent, l log.Component) bool {
	if setup.IsCLCRunner(config) {
		return false
	}

	runInCoreAgent := config.GetBool("process_config.run_in_core_agent.enabled")

	var processEnabled bool
	for _, check := range checkComponents {
		if slices.Contains(processCheckNames, check.Object().Name()) && check.Object().IsEnabled() {
			processEnabled = true
		}
	}

	switch flavor.GetFlavor() {
	case flavor.ProcessAgent:
		if runInCoreAgent {
			l.Info("The process checks will run in the core agent via the process-component")
		} else if processEnabled {
			l.Info("Process/Container Collection in the Process Agent will be deprecated in a future release " +
				"and will instead be run in the Core Agent.")
		}
		return !runInCoreAgent
	case flavor.DefaultAgent:
		return runInCoreAgent
	default:
		return false
	}
}

// Enabled determines whether the process agent is enabled based on the configuration.
// On AIX, process checks run in the core agent when process_config.run_in_core_agent.enabled is true.
func Enabled(config config.Component, checkComponents []types.CheckComponent, l log.Component) bool {
	Once.Do(func() {
		enabled = enabledHelper(config, checkComponents, l)
	})
	return enabled
}
