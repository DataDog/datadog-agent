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
)

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
	lines           []scannedLine // classified ADP output; local only
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
// Only bounded enums reach Datadog: label values come from the finding constants, never from
// ADP's output, which would both explode cardinality and risk shipping operator-controlled
// text.
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
		finding: tlm.NewCounter(telemetrySubsystem, metricFinding, []string{labelFinding},
			"Individual problems observed during an Agent Data Plane preflight-mode run"),
		duration: tlm.NewGauge(telemetrySubsystem, metricDuration, nil,
			"Wall-clock seconds the most recent Agent Data Plane preflight-mode run took"),
	}
}

// report ships the outcome.
//
// ADP's actual error text is not shipped anywhere yet: the agent telemetry error tracking
// pipeline intentionally carries no message field, because a message may contain
// operator-controlled text. Sending it needs a new event type in defaultProfiles.yaml plus a
// matching backend schema, agreed with the team that owns the pipeline. Until then the
// messages are logged locally — reachable in a flare — while the bounded counters carry the
// signal that reaches Datadog.
//
// TODO(DADP-xxx): ship o.lines via agenttelemetry SendEvent once the event type is agreed.
func (r *reporter) report(o *outcome) {
	r.duration.Set(o.durationSeconds)
	r.result.Inc(o.result())
	for _, f := range o.findings {
		r.finding.Inc(string(f))
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

	for _, l := range o.lines {
		r.log.Warnf("Agent Data Plane preflight mode observed %s from %s: %s", l.Level, l.Target, l.Signature)
	}
}
