// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows || darwin

package preflightmodeimpl

import (
	"strings"

	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// finding is a bounded enum describing one thing that went wrong during a preflight mode run.
// Values are shipped as a telemetry label, so this set must stay small and must never
// contain anything derived from ADP's output.
type finding string

const (
	findingSpawnFailed   finding = "spawn_failed"    // ADP could not be started
	findingProbeFailed   finding = "probe_failed"    // the probe metric never made it into ADP
	findingExitedEarly   finding = "exited_early"    // ADP exited before we asked it to
	findingStopTimeout   finding = "stop_timeout"    // ADP had to be killed
	findingErrorsInLog   finding = "errors_in_log"   // ADP logged an error
	findingWarningsInLog finding = "warnings_in_log" // ADP logged a warning we did not cause
	findingOutputDropped finding = "output_dropped"  // the capture buffer overflowed
	findingInterrupted   finding = "interrupted"     // the Agent shut down mid-run
)

// resultClean is the result label used when a run produced no findings at all.
const resultClean = "clean"

// allFindings exists so a test can assert the set has not grown without the agent telemetry
// profile being updated to match.
var allFindings = []finding{
	findingSpawnFailed,
	findingProbeFailed,
	findingExitedEarly,
	findingStopTimeout,
	findingErrorsInLog,
	findingWarningsInLog,
	findingOutputDropped,
	findingInterrupted,
}

// Metric and label names. These are duplicated in the agent telemetry profile in
// comp/core/agenttelemetry/impl/defaultProfiles.yaml, where they must be allowlisted —
// including each label — or nothing is shipped. TestTelemetryNamesAreStable is the tripwire.
const (
	telemetrySubsystem = "data_plane"

	metricResult   = "preflight_mode_result"
	metricFinding  = "preflight_mode_finding"
	metricDuration = "preflight_mode_duration_seconds"

	labelResult  = "result"
	labelFinding = "finding"
	// labelSourceFile and labelSourceLine carry the log site a finding came from, and are only
	// populated for the findings that come from an ADP log record. See sourceLocation.
	labelSourceFile = "source_file"
	labelSourceLine = "source_line"
)

// maxReportedLocations bounds how many distinct log sites one finding is reported with.
//
// Each location is a separate timeseries on the finding metric, so this is a cardinality bound
// as much as a payload one: a run that logged from hundreds of distinct sites would otherwise
// turn a single finding into hundreds of points. A failing ADP startup logs from a handful of
// sites — the captured fixtures show two or three — so this is well clear of the normal case.
const maxReportedLocations = 10

// locationNone is the location reported for a finding that did not come from an ADP log record.
//
// The metric's label set is fixed, so every point carries both location labels whether or not
// the finding has a location to put in them, and an empty value is how "not applicable" is
// expressed. Deliberately not a placeholder string, which could not be told apart from a
// location ADP actually reported.
var locationNone = sourceLocation{}

// locationOverflow stands in for every distinct location past maxReportedLocations, so the
// points reported for a finding still add up to the number of distinct locations observed
// rather than the surplus being silently dropped.
var locationOverflow = sourceLocation{file: "<other>", line: "<other>"}

// probeMetricName is the throwaway metric pushed through ADP.
//
// The n_o_i_n_d_e_x. prefix is what keeps the point out of the customer's indexed metrics. The
// DogStatsD text protocol has no no-index flag — MetricSample.NoIndex is only reachable
// in-process — so a name prefix is the only option. The prefix must stay leading, which is why
// the generated config clears statsd_metric_namespace.
const probeMetricName = "n_o_i_n_d_e_x.datadog.agent.data_plane.preflight_mode.probe"

// outcome is everything a single preflight mode run produced.
type outcome struct {
	findings        []finding
	records         []logRecord // parsed ADP output; local only
	durationSeconds float64
}

// add records a finding, ignoring duplicates.
func (o *outcome) add(f finding) {
	for _, existing := range o.findings {
		if existing == f {
			return
		}
	}
	o.findings = append(o.findings, f)
}

// reportedLocations returns the log sites the finding is reported with, one point each.
//
// Only the two log findings have a location at all: every other finding is something the
// pre-flight itself observed about the process, not something ADP logged from a known place in
// its source.
func (o *outcome) reportedLocations(f finding) []sourceLocation {
	var locations []sourceLocation
	switch f {
	case findingErrorsInLog:
		locations = locationsOf(o.records, isError)
	case findingWarningsInLog:
		locations = locationsOf(o.records, isUnexpectedWarning)
	default:
		return []sourceLocation{locationNone}
	}

	// A log finding is only recorded when a matching record exists, so an empty set here means
	// the two have fallen out of step. Report the finding without a location rather than not at
	// all: the finding is the signal, and the location only says where to look.
	if len(locations) == 0 {
		return []sourceLocation{locationNone}
	}
	if len(locations) <= maxReportedLocations {
		return locations
	}

	capped := make([]sourceLocation, 0, len(locations))
	capped = append(capped, locations[:maxReportedLocations]...)
	for range locations[maxReportedLocations:] {
		capped = append(capped, locationOverflow)
	}
	return capped
}

// result is the single value reported for the run. The first finding wins, since findings are
// recorded in the order they occur and the earliest is the most explanatory.
func (o *outcome) result() string {
	if len(o.findings) == 0 {
		return resultClean
	}
	return string(o.findings[0])
}

// reporter ships the outcome of a preflight mode run.
//
// What reaches Datadog stays bounded: the result and finding labels come from the constants
// above, and the only values read out of ADP's output are the log site a finding came from —
// a file path and a line number, which ADP's logger fills in from file!() and line!(), so they
// are compile-time constants of its build rather than anything operator-controlled. Both are
// validated and the number of distinct locations per finding is capped, because cardinality here
// is a cost paid by every Agent in the fleet. Nothing derived from a log *message* is shipped.
type reporter struct {
	log      logcomp.Component
	result   telemetry.Counter
	finding  telemetry.Counter
	duration telemetry.Gauge
}

func newReporter(log logcomp.Component, tlm telemetry.Component) *reporter {
	return &reporter{
		log: log,
		result: tlm.NewCounter(telemetrySubsystem, metricResult, []string{labelResult},
			"Outcome of the most recent Agent Data Plane preflight-mode run"),
		finding: tlm.NewCounter(telemetrySubsystem, metricFinding,
			[]string{labelFinding, labelSourceFile, labelSourceLine},
			"Individual problems observed during an Agent Data Plane preflight-mode run, tagged with the Agent Data Plane source location that logged the problem where it has one"),
		duration: tlm.NewGauge(telemetrySubsystem, metricDuration, nil,
			"Wall-clock seconds the most recent Agent Data Plane preflight-mode run took"),
	}
}

// report ships the outcome.
//
// The log site of an error or warning is shipped as a tag on the finding metric; its text is
// not. ADP's actual error text is not shipped anywhere yet: the agent telemetry error tracking
// pipeline intentionally carries no message field, because a message may contain
// operator-controlled text. Sending it needs a new event type in defaultProfiles.yaml plus a
// matching backend schema, agreed with the team that owns the pipeline. Until then the
// messages are logged locally — reachable in a flare — while the bounded counters carry the
// signal that reaches Datadog.
//
// TODO(DADP-xxx): ship o.records via agenttelemetry SendEvent once the event type is agreed.
func (r *reporter) report(o *outcome) {
	r.duration.Set(o.durationSeconds)
	r.result.Inc(o.result())
	for _, f := range o.findings {
		// One point per log site for the findings that have one, so that the same failure can be
		// counted across the fleet by where in ADP it was logged from. A finding with several
		// sites is therefore several points: the per-site count is what identifies the failure,
		// while the count of runs that hit the finding at all comes from the result metric.
		for _, loc := range o.reportedLocations(f) {
			r.finding.Inc(string(f), loc.file, loc.line)
		}
	}

	if len(o.findings) == 0 {
		r.log.Infof("Agent Data Plane preflight mode completed cleanly in %.1fs", o.durationSeconds)
	} else {
		names := make([]string, 0, len(o.findings))
		for _, f := range o.findings {
			names = append(names, string(f))
		}
		r.log.Warnf("Agent Data Plane preflight mode completed in %.1fs with %d finding(s): %s",
			o.durationSeconds, len(o.findings), strings.Join(names, ", "))
	}

	// Only the notable records are surfaced at this level. The rest were retained as context
	// for them and are already in the Agent log at debug, where the capture mirrored them.
	for _, rec := range o.records {
		if !rec.notable() {
			continue
		}
		r.log.Warnf("Agent Data Plane preflight mode observed %s from %s (%s:%s): %s",
			rec.Level, rec.Target, rec.SourceFile, rec.SourceLine, rec.Signature)
	}
}
