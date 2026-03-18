// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package agent

import (
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
	Once sync.Once
)

func enabledHelper(config config.Component, checkComponents []types.CheckComponent, l log.Component) bool {
	// never run the process component in the cluster worker
	if setup.IsCLCRunner(config) {
		return false
	}

	var npmEnabled bool
	for _, check := range checkComponents {
		if check.Object().Name() == checks.ConnectionsCheckName && check.Object().IsEnabled() {
			npmEnabled = true
		}
	}

	switch flavor.GetFlavor() {
	case flavor.ProcessAgent:
		// Process checks always run in the core agent on Linux.
		// The process-agent is only needed if NPM (connections check) is enabled.
		if npmEnabled {
			l.Info("Network Performance Monitoring is not supported in the core agent. " +
				"The process-agent will be enabled as a standalone agent")
		}
		l.Info("The process checks will run in the core agent via the process-component")
		return npmEnabled
	case flavor.DefaultAgent:
		if npmEnabled {
			l.Info("Network Performance Monitoring is not supported in the core agent. " +
				"The process-agent will be enabled as a standalone agent to collect network performance metrics.")
		}
		return true
	default:
		return false
	}
}

// Enabled determines whether the process agent component is enabled.
// Enabled will only be run once, to prevent duplicate logging.
// On Linux, process checks always run in the core agent. The standalone process-agent
// is only needed when the connections/NPM check is enabled.
func Enabled(config config.Component, checkComponents []types.CheckComponent, l log.Component) bool {
	Once.Do(func() {
		enabled = enabledHelper(config, checkComponents, l)
	})
	return enabled
}
