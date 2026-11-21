// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package intakeroutingimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
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

// telemetryTypes lists every telemetry type a consumer can register. The metrics are created for
// each of them up front.
var telemetryTypes = []intakerouting.TelemetryType{intakerouting.Logs}

// Every value of the tags below. Each combination is created at 0 up front.
var (
	queryStatuses     = []string{statusSuccess, statusNotOffered, statusNetworkError, statusTimeout, statusHTTPError, statusInvalidResponse}
	preflightStatuses = []string{preflightSuccess, preflightFailure}
	switchDirections  = []string{directionToEnhanced, directionToDefault, directionEnhancedToEnhanced}
	rejectReasons     = []string{rejectInvalidHost, rejectAbsent}
	pathReasons       = []string{pathPrivateAddress, pathProxied, pathLookupFailed}
)

// newRoutingTelemetry creates the metrics, with every combination of tag values at 0. They are only
// created when intake routing is enabled, so they show up as soon as it is, whether or not any
// endpoint was registered, rather than when something first happens.
func newRoutingTelemetry(tm telemetry.Component) *routingTelemetry {
	t := &routingTelemetry{
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
	for _, status := range queryStatuses {
		t.queries.InitializeToZero(status)
	}
	for _, status := range preflightStatuses {
		t.preflights.InitializeToZero(status)
	}
	for _, typ := range telemetryTypes {
		for _, direction := range switchDirections {
			t.switches.InitializeToZero(string(typ), direction)
		}
		for _, reason := range rejectReasons {
			t.rejectedAnswers.InitializeToZero(string(typ), reason)
		}
		for _, reason := range pathReasons {
			t.skippedEndpoints.InitializeToZero(string(typ), reason)
		}
		t.activeOverrides.Set(0, string(typ))
	}
	return t
}
