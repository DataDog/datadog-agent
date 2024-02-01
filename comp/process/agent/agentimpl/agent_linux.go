// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package agentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// agentEnabled determines whether the process agent is enabled based on the configuration
// The process-agent is enabled when the flavor is set to ProcessAgent in all non-linux platforms
func agentEnabled(p processAgentParams) bool {
	runInCoreAgent := p.Config.GetBool("process_config.run_in_core_agent.enabled")

	switch flavor.GetFlavor() {
	case flavor.ProcessAgent:
		return !runInCoreAgent
	case flavor.DefaultAgent:
		for _, check := range p.Checks {
			// the connections check is not supported in the core agent
			if check.Object().Name() == checks.ConnectionsCheckName && check.Object().IsEnabled() {
				return false
			}
		}
		return runInCoreAgent
	default:
		return false
	}
}
