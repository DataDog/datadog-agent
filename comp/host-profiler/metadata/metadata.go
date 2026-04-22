// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package metadata provides runtime identity metadata (name, version, mode)
// for the host-profiler.
package metadata

import "github.com/DataDog/datadog-agent/pkg/version"

const (
	ProfilerNameKey    = "profiler_name"
	ProfilerVersionKey = "profiler_version"
	ProfilerModeKey    = "datadog.hostprofiler.mode"
)

const (
	ProfilerName           = "host-profiler"
	ProfilerModeBundled    = "bundled"
	ProfilerModeStandalone = "standalone"
)

var ProfilerVersion = version.AgentVersion
