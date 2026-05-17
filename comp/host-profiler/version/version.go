// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package version provides version information for the host profiler.
package version

import "github.com/DataDog/datadog-agent/pkg/version"

// Datadog semantic conventions (bundled mode).
const (
	BundledProfilerName  = "host-profiler-bundled"
	DDProfilerNameKey    = "profiler_name"
	DDProfilerVersionKey = "profiler_version"
)

// OpenTelemetry semantic conventions (standalone mode).
const (
	StandaloneProfilerName = "host-profiler-standalone"
	OTelProfilerNameKey    = "telemetry.distro.name"
	OTelProfilerVersionKey = "telemetry.distro.version"
)

var ProfilerVersion = version.AgentVersion
