// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package profiler provides a flare folder containing the output of various agent's pprof servers
package profiler

// team: agent-shared-components

import flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"

// Component is the component type
type Component interface {
	ReadProfileData(seconds int, logFunc func(log string, params ...interface{}) error) (flaretypes.ProfileData, error)
}
