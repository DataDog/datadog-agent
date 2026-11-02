// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package intakeroutingimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// telemetrySubsystem prefixes every metric. Agent telemetry (COAT) names them
// "intake_routing.<name>", so the defaults (double underscore separator) must be kept.
const telemetrySubsystem = "intake_routing"

// Tag values. Every tag has a fixed set of values: no tag carries a host or an API key. The
// "status" values of the queries counter are the query statuses in client.go, and the "reason"
// values of the skipped_endpoints counter are the path reasons in path.go.
const (
	preflightSuccess = "success"
	preflightFailure = "failure"

	directionToEnhanced         = "to_enhanced"
	directionToDefault          = "to_default"
	directionEnhancedToEnhanced = "enhanced_to_enhanced"

	rejectInvalidHost = "invalid_host"
	rejectAbsent      = "absent"
)

type routingTelemetry struct {
	queries          telemetry.Counter
	preflights       telemetry.Counter
	switches         telemetry.Counter
	rejectedAnswers  telemetry.Counter
	skippedEndpoints telemetry.Counter
	activeOverrides  telemetry.Gauge
}

func newRoutingTelemetry(tm telemetry.Component) *routingTelemetry {
	return &routingTelemetry{
		queries: tm.NewCounter(telemetrySubsystem, "queries",
			[]string{"status"},
			"Intake routing queries sent to the Datadog backend, by outcome."),
		preflights: tm.NewCounter(telemetrySubsystem, "preflights",
			[]string{"status"},
			"Pre-flight checks of hosts returned by the intake routing backend, by outcome."),
		switches: tm.NewCounter(telemetrySubsystem, "switches",
			[]string{"telemetry_type", "direction"},
			"Endpoints switched to another host by intake routing."),
		rejectedAnswers: tm.NewCounter(telemetrySubsystem, "rejected_answers",
			[]string{"telemetry_type", "reason"},
			"Intake routing answers that were not applied, by reason."),
		skippedEndpoints: tm.NewCounter(telemetrySubsystem, "skipped_endpoints",
			[]string{"telemetry_type", "reason"},
			"Endpoints intake routing did not route because the Agent does not reach their configured host directly, by reason."),
		activeOverrides: tm.NewGauge(telemetrySubsystem, "active_overrides",
			[]string{"telemetry_type"},
			"Endpoints currently sent to a host chosen by intake routing instead of their configured host."),
	}
}
