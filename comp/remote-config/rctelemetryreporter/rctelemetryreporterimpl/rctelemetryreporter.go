// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rctelemetryreporterimpl provides a DdRcTelemetryReporter that sends RC-specific metrics to the DD backend.
package rctelemetryreporterimpl

import (
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newDdRcTelemetryReporter),
	)
}

// DdRcTelemetryReporter is a datadog-agent telemetry counter for RC cache bypass metrics. It implements the RcTelemetryReporter interface.
type DdRcTelemetryReporter struct {
	BypassRateLimitCounter telemetry.Counter
	BypassTimeoutCounter   telemetry.Counter
}

// IncRateLimit increments the DdRcTelemetryReporter BypassRateLimitCounter counter.
func (r *DdRcTelemetryReporter) IncRateLimit() {
	r.BypassRateLimitCounter.Inc()
}

// IncTimeout increments the DdRcTelemetryReporter BypassTimeoutCounter counter.
func (r *DdRcTelemetryReporter) IncTimeout() {
	r.BypassTimeoutCounter.Inc()
}

// newDdRcTelemetryReporter creates a new Remote Config telemetry reporter for sending RC metrics to Datadog
func newDdRcTelemetryReporter() rctelemetryreporter.Component {
	commonOpts := telemetry.Options{NoDoubleUnderscoreSep: true}
	return &DdRcTelemetryReporter{
		BypassRateLimitCounter: telemetry.NewCounterWithOpts(
			"remoteconfig",
			"cache_bypass_ratelimiter_skip",
			[]string{},
			"Number of Remote Configuration cache bypass requests skipped by rate limiting.",
			commonOpts,
		),
		BypassTimeoutCounter: telemetry.NewCounterWithOpts(
			"remoteconfig",
			"cache_bypass_timeout",
			[]string{},
			"Number of Remote Configuration cache bypass requests that timeout.",
			commonOpts,
		),
	}
}
