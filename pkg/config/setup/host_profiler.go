// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package setup defines the configuration of the agent
package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	hostProfilerSection        = "host_profiler"
	hostProfilerDebugSection   = hostProfilerSection + ".debug"
	HostProfilerDebugVerbosity = hostProfilerDebugSection + ".verbosity"
)

func setupHostProfiler(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault(HostProfilerDebugVerbosity, "none", "DD_HOST_PROFILER_DEBUG_VERBOSITY")
}
