// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build aix

package agent

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

var (
	// enabled variable to ensure value returned by Enabled() persists when Enabled() is called multiple times
	enabled bool
	// Once module variable, exported for testing
	Once sync.Once
)

func enabledHelper(config config.Component, _ []types.CheckComponent, l log.Component) bool {
	// never run the process component in the cluster worker
	if setup.IsCLCRunner(config) {
		return false
	}

	switch flavor.GetFlavor() {
	case flavor.ProcessAgent:
		// Process checks always run in the core agent on AIX.
		// There is no NPM or other standalone-only check on AIX.
		l.Info("The process checks will run in the core agent via the process-component")
		return false
	case flavor.DefaultAgent:
		return true
	default:
		return false
	}
}

// Enabled determines whether the process agent component is enabled.
// Enabled will only be run once, to prevent duplicate logging.
// On AIX, process checks always run in the core agent.
func Enabled(config config.Component, checkComponents []types.CheckComponent, l log.Component) bool {
	Once.Do(func() {
		enabled = enabledHelper(config, checkComponents, l)
	})
	return enabled
}
