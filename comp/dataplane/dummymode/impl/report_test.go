// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package dummymodeimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFindingsAreAllowlisted is a tripwire, not a check of behaviour.
//
// A finding only reaches Datadog if data_plane.dummy_mode_finding is listed in
// comp/core/agenttelemetry/impl/defaultProfiles.yaml *with* "finding" in its
// preserve_tags — an unlisted metric is dropped and an unlisted label is silently
// stripped, with its timeseries summed into the others. That file cannot be read from this
// package (it is embedded in another module's package), so the set is pinned here instead.
//
// If this test fails you have added or renamed a finding. Update the expected set below,
// and make sure the agent telemetry profile still covers it — see
// TestDataPlaneDummyModeProfile in comp/core/agenttelemetry/impl/agenttelemetry_test.go.
func TestFindingsAreAllowlisted(t *testing.T) {
	expected := []finding{
		"spawn_failed",
		"probe_failed",
		"exited_early",
		"stop_timeout",
		"errors_in_log",
		"warnings_in_log",
		"output_dropped",
		"interrupted",
	}
	assert.ElementsMatch(t, expected, allFindings)
}

// TestTelemetryNamesAreStable pins the metric and label names for the same reason: they
// are duplicated in the agent telemetry profile.
func TestTelemetryNamesAreStable(t *testing.T) {
	assert.Equal(t, "data_plane", telemetrySubsystem)
	assert.Equal(t, "dummy_mode_result", metricResult)
	assert.Equal(t, "dummy_mode_finding", metricFinding)
	assert.Equal(t, "dummy_mode_duration_seconds", metricDuration)
	assert.Equal(t, "result", labelResult)
	assert.Equal(t, "finding", labelFinding)
}

// TestProbeMetricName pins the probe name. The n_o_i_n_d_e_x. prefix is what keeps the
// probe out of the customer's indexed metrics, and it must be the very first thing in the
// name — see the comment on probeMetricName.
func TestProbeMetricName(t *testing.T) {
	assert.Equal(t, "n_o_i_n_d_e_x.datadog.agent.data_plane.dummy_mode.probe", probeMetricName)
}
