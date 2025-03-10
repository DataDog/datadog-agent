// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package profiler provides a flare folder containing the output of various agent's pprof servers
package profiler

// team: agent-configuration

import flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"

// Component is the component type
type Component interface {
	// ReadProfileData Gathers and returns pprof server output for a variety of agent services.
	//
	// Will always attempt to read the pprof of core-agent and security-agent, and will optionally try to read information for
	// process-agent, trace-agent, and system-probe if those systems are detected as enabled.
	//
	// This function is exposed via the public api to support the flare generation cli command. While the goal
	// is to move the profiling component completely into a flare provider, the existing architecture
	// expects an explicit and pre-emptive profiling run before the flare logic is properly called.
	ReadProfileData(seconds int, logFunc func(log string, params ...interface{}) error) (flaretypes.ProfileData, error)
}
