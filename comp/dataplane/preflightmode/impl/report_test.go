// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFindingsAreAllowlisted is a tripwire, not a check of behaviour.
//
// A finding only reaches Datadog if data_plane.preflight_mode_finding is listed in
// comp/core/agenttelemetry/impl/defaultProfiles.yaml *with* "finding" in its
// preserve_tags — an unlisted metric is dropped and an unlisted label is silently
// stripped, with its timeseries summed into the others. That file cannot be read from this
// package (it is embedded in another module's package), so the set is pinned here instead.
//
// If this test fails you have added or renamed a finding. Update the expected set below,
// and make sure the agent telemetry profile still covers it — see
// TestDataPlanePreflightModeProfile in comp/core/agenttelemetry/impl/agenttelemetry_test.go.
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

func TestOutcomeResult(t *testing.T) {
	t.Run("no findings is clean", func(t *testing.T) {
		assert.Equal(t, resultClean, (&outcome{}).result())
	})

	t.Run("the first finding wins", func(t *testing.T) {
		o := &outcome{}
		o.add(findingProbeFailed)
		o.add(findingErrorsInLog)
		assert.Equal(t, string(findingProbeFailed), o.result())
	})

	t.Run("findings are deduplicated", func(t *testing.T) {
		o := &outcome{}
		o.add(findingErrorsInLog)
		o.add(findingErrorsInLog)
		assert.Equal(t, []finding{findingErrorsInLog}, o.findings)
	})
}

// TestTelemetryNamesAreStable pins the metric and label names for the same reason: they
// are duplicated in the agent telemetry profile.
func TestTelemetryNamesAreStable(t *testing.T) {
	assert.Equal(t, "data_plane", telemetrySubsystem)
	assert.Equal(t, "preflight_mode_result", metricResult)
	assert.Equal(t, "preflight_mode_finding", metricFinding)
	assert.Equal(t, "preflight_mode_duration_seconds", metricDuration)
	assert.Equal(t, "result", labelResult)
	assert.Equal(t, "finding", labelFinding)
	assert.Equal(t, "source_file", labelSourceFile)
	assert.Equal(t, "source_line", labelSourceLine)
}

// TestReportedLocations covers what the finding metric is tagged with. Only the two findings
// that come from an ADP log record have a log site; everything else is something the pre-flight
// observed about the process rather than something ADP logged.
func TestReportedLocations(t *testing.T) {
	// errAt builds an error record logged from one site, with the message under our control so
	// that two records from the same site can be told apart.
	errAt := func(line sourceLine, message string) logRecord {
		return logRecord{
			Level:      levelError,
			Target:     "agent_data_plane",
			Signature:  message,
			SourceFile: "bin/agent-data-plane/src/main.rs",
			SourceLine: line,
		}
	}

	t.Run("a finding with no log site reports empty labels", func(t *testing.T) {
		o := &outcome{records: []logRecord{errAt("195", "boom")}}
		assert.Equal(t, []sourceLocation{locationNone}, o.reportedLocations(findingProbeFailed))
	})

	t.Run("errors report every distinct log site", func(t *testing.T) {
		o := &outcome{records: []logRecord{
			errAt("195", "could not start"),
			// Same file, different line: a different log site.
			errAt("212", "could not stop"),
			// The first site again with a different message: one site, so one point.
			errAt("195", "could not start, again"),
			// A warning is not an error.
			{Level: levelWarn, Target: "t", Signature: "w", SourceFile: "lib/saluki-core/src/lib.rs", SourceLine: "9"},
		}}

		assert.Equal(t, []sourceLocation{
			{file: "bin/agent-data-plane/src/main.rs", line: "195"},
			{file: "bin/agent-data-plane/src/main.rs", line: "212"},
		}, o.reportedLocations(findingErrorsInLog))
	})

	t.Run("warnings report only the ones preflight mode did not provoke", func(t *testing.T) {
		expected, ok := parseRecord(realWarnStandalone)
		require.True(t, ok)
		unexpected, ok := parseRecord(realWarnInvalidAPIKey)
		require.True(t, ok)

		o := &outcome{records: []logRecord{expected, unexpected}}
		assert.Equal(t, []sourceLocation{unexpected.location()}, o.reportedLocations(findingWarningsInLog))
	})

	t.Run("a log finding with no matching record still reports", func(t *testing.T) {
		// Defensive: the finding and the records have fallen out of step. The finding is the
		// signal, so it must still ship.
		o := &outcome{}
		assert.Equal(t, []sourceLocation{locationNone}, o.reportedLocations(findingErrorsInLog))
	})

	t.Run("the surplus past the cap collapses onto one location", func(t *testing.T) {
		var records []logRecord
		const sites = maxReportedLocations + 3
		for i := 0; i < sites; i++ {
			records = append(records, errAt(sourceLine(strconv.Itoa(i)), "boom"))
		}

		locations := (&outcome{records: records}).reportedLocations(findingErrorsInLog)
		require.Len(t, locations, sites, "the points must still add up to the sites observed")
		assert.NotContains(t, locations[:maxReportedLocations], locationOverflow,
			"the earliest sites are the ones kept")
		for _, loc := range locations[maxReportedLocations:] {
			assert.Equal(t, locationOverflow, loc)
		}
	})
}

// TestProbeMetricName pins the probe name. The n_o_i_n_d_e_x. prefix is what keeps the
// probe out of the customer's indexed metrics, and it must be the very first thing in the
// name — see the comment on probeMetricName.
func TestProbeMetricName(t *testing.T) {
	assert.Equal(t, "n_o_i_n_d_e_x.datadog.agent.data_plane.preflight_mode.probe", probeMetricName)
}
