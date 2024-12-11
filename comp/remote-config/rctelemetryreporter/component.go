// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rctelemetryreporter provides a component that sends RC-specific metrics to the DD backend.
package rctelemetryreporter

// team: remote-config

// Component is the component type.
type Component interface {
	// IncTimeout increments the DdRcTelemetryReporter BypassTimeoutCounter counter.
	IncTimeout()
	// IncRateLimit increments the DdRcTelemetryReporter BypassRateLimitCounter counter.
	IncRateLimit()
}
