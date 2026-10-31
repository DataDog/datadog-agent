// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

const (
	// ProfilingEndpointPrefix is the URL prefix for the profiling intake.
	ProfilingEndpointPrefix = "https://intake.profile."
	// ProfilingEndpointPath is the API path for the profiling intake.
	ProfilingEndpointPath = "/api/v2/profile"

	// DebuggerLogsEndpointPrefix is the URL prefix for the debugger logs intake.
	DebuggerLogsEndpointPrefix = "https://http-intake.logs."
	// DebuggerLogsEndpointPath is the API path for the debugger logs intake.
	DebuggerLogsEndpointPath = "/api/v2/logs"

	// DebuggerIntakeEndpointPrefix is the URL prefix for the debugger diagnostics/symbol database intake.
	DebuggerIntakeEndpointPrefix = "https://debugger-intake."
	// DebuggerIntakeEndpointPath is the API path for the debugger diagnostics/symbol database intake.
	DebuggerIntakeEndpointPath = "/api/v2/debugger"

	// OpenLineageEndpointPrefix is the URL prefix for the openlineage intake.
	OpenLineageEndpointPrefix = "https://data-obs-intake."
	// OpenLineageEndpointPath is the API path for the openlineage intake.
	OpenLineageEndpointPath = "/api/v1/lineage"
)
