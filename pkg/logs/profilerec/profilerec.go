// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package profilerec recommends a logs_config.profile from pipeline saturation and recent log loss.
package profilerec

import (
	"expvar"
	"fmt"
	"strings"
	"sync"
	"time"

	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Profile names guaranteed to exist in the pkg/config/setup catalog.
const (
	ProfileHighThroughput  = "high-throughput"
	ProfileHighConcurrency = "high-concurrency"
)

// Stable reason codes (used by the Fleet UI).
const (
	ReasonSendStageSaturatedHighLatency = "send_stage_saturated_high_latency"
	ReasonSendStageSaturated            = "send_stage_saturated"
	ReasonProcessorSaturated            = "processor_saturated"
	ReasonStrategySaturated             = "strategy_saturated"
	ReasonPipelineSaturated             = "pipeline_saturated"
)

// SenderLatencyHighThresholdMs is the intake round-trip latency above which a send bottleneck is treated as latency-bound.
const SenderLatencyHighThresholdMs = 250

// LossRecencyWindow is how long after the last observed loss the pipeline still counts as actively losing logs.
const LossRecencyWindow = 5 * time.Minute

// Stage is the minimal per-component view the recommender needs.
type Stage struct {
	Name                string
	CurrentlySaturated  bool
	Saturated1mSeconds  int64
	Saturated30mSeconds int64
}

// StagesFromBackpressure adapts comp/logs-library/metrics components.
func StagesFromBackpressure(comps []logsmetrics.ComponentBackpressure) []Stage {
	stages := make([]Stage, 0, len(comps))
	for _, c := range comps {
		stages = append(stages, Stage{
			Name:                c.Component,
			CurrentlySaturated:  c.CurrentlySaturated,
			Saturated1mSeconds:  c.Saturated1mSeconds,
			Saturated30mSeconds: c.Saturated30mSeconds,
		})
	}
	return stages
}

// Recommendation is a profile suggestion with a stable code and a human-readable reason.
type Recommendation struct {
	Profile    string
	ReasonCode string
	Reason     string
	Bottleneck string
}

// Signals are the recent-loss inputs.
type Signals struct {
	MissedRecently, Delivering bool
	SenderLatencyMs            int64
}

// componentSortOrder defines the canonical downstream ordering of pipeline components.
var componentSortOrder = map[string]int{
	"processor": 0,
	"strategy":  1,
	"worker":    2,
}

// ComponentRank orders pipeline components upstream to downstream; destinations and unknown components come last.
func ComponentRank(name string) int {
	if r, ok := componentSortOrder[name]; ok {
		return r
	}
	return 10
}

// IsSendStage reports whether the component is part of the network send stage (workers, the sender, or a destination).
func IsSendStage(component string) bool {
	return component == "worker" || component == logsmetrics.SenderTlmName || strings.HasPrefix(component, "destination_")
}

// Bottleneck returns the most-downstream currently-saturated stage, falling back to recent (1m/30m) saturation, or "" if none.
func Bottleneck(stages []Stage) string {
	if c := mostDownstreamSaturated(stages, func(s Stage) bool { return s.CurrentlySaturated }); c != "" {
		return c
	}
	return mostDownstreamSaturated(stages, func(s Stage) bool {
		return s.Saturated1mSeconds > 0 || s.Saturated30mSeconds > 0
	})
}

// mostDownstreamSaturated returns the deepest stage for which sat() is true, or "".
// Backpressure propagates upstream, so the deepest saturated stage is the true bottleneck.
func mostDownstreamSaturated(stages []Stage, sat func(Stage) bool) string {
	best := ""
	bestRank := -1
	for _, s := range stages {
		if !sat(s) {
			continue
		}
		if r := ComponentRank(s.Name); r > bestRank {
			bestRank = r
			best = s.Name
		}
	}
	return best
}

// ForBottleneck maps the bottleneck component to a profile, a stable reason code and a one-line diagnosis.
func ForBottleneck(component string, latencyMs int64) (profile, reasonCode, reason string) {
	switch {
	case component == "processor":
		return ProfileHighThroughput, ReasonProcessorSaturated, "The logs pipeline is bottlenecked at the processor stage, which is CPU-bound."
	case component == "strategy":
		return ProfileHighThroughput, ReasonStrategySaturated, "The logs pipeline is bottlenecked at the compression and batching stage, which is CPU-bound."
	case IsSendStage(component):
		if latencyMs >= SenderLatencyHighThresholdMs {
			return ProfileHighConcurrency, ReasonSendStageSaturatedHighLatency, fmt.Sprintf("The logs pipeline is bottlenecked at the network send stage, with high intake latency (%dms).", latencyMs)
		}
		return ProfileHighConcurrency, ReasonSendStageSaturated, "The logs pipeline is bottlenecked at the network send stage."
	default:
		return ProfileHighThroughput, ReasonPipelineSaturated, "The logs pipeline is saturated."
	}
}

// Recommend suggests a profile only when bytes were recently missed; saturation merely localizes the bottleneck.
// Destination drops are not a signal: they count permanent send errors, which no profile fixes.
// It returns nil when nothing is being lost, when no profile would help, or when activeProfile already covers the suggestion.
func Recommend(stages []Stage, activeProfile string, s Signals) *Recommendation {
	if !s.MissedRecently {
		return nil
	}

	bottleneck := Bottleneck(stages)
	if bottleneck == "" || (IsSendStage(bottleneck) && !s.Delivering) {
		return nil
	}

	profile, code, reason := ForBottleneck(bottleneck, s.SenderLatencyMs)
	// The coverage check compares catalog profiles by name; an explicit override always wins over a profile,
	// so the only way effective coverage breaks is one where switching would not change that knob anyway.
	if profile == "" || profile == activeProfile || pkgconfigsetup.LogsPerformanceProfileCovers(activeProfile, profile) {
		return nil
	}
	return &Recommendation{
		Profile:    profile,
		ReasonCode: code,
		Reason:     "Logs are being lost. " + reason,
		Bottleneck: bottleneck,
	}
}

// Counters is a snapshot of the logs expvars.
type Counters struct {
	Dropped, Missed, Processed, Sent, Errors, SenderLatencyMs int64
}

// ReadCounters reads the counters from an expvar map shaped like logsmetrics.LogsExpvars; a nil map yields zeros.
func ReadCounters(m *expvar.Map) Counters {
	if m == nil {
		return Counters{}
	}
	return Counters{
		Dropped:         droppedTotal(m),
		Missed:          intVar(m, "BytesMissed"),
		Processed:       intVar(m, "LogsProcessed"),
		Sent:            intVar(m, "LogsSent"),
		Errors:          intVar(m, "DestinationErrors"),
		SenderLatencyMs: intVar(m, "SenderLatency"),
	}
}

func intVar(m *expvar.Map, key string) int64 {
	if v, ok := m.Get(key).(*expvar.Int); ok && v != nil {
		return v.Value()
	}
	return 0
}

// droppedTotal sums logs dropped across all destinations.
func droppedTotal(m *expvar.Map) int64 {
	dm, ok := m.Get("DestinationLogsDropped").(*expvar.Map)
	if !ok || dm == nil {
		return 0
	}
	var total int64
	dm.Do(func(kv expvar.KeyValue) {
		if v, ok := kv.Value.(*expvar.Int); ok && v != nil {
			total += v.Value()
		}
	})
	return total
}

// LossWindow turns the monotonic dropped / missed / processed / sent counters into recent-activity signals.
// Each Observe records when a counter last increased, so a stale historical loss ages out instead of keeping a
// recommendation pinned on, and delivery is judged on the latest interval rather than lifetime totals.
type LossWindow struct {
	mu                      sync.Mutex
	seeded                  bool
	lastDropped, lastMissed int64
	lastProcessed, lastSent int64
	lastErrors              int64
	droppedAt, missedAt     time.Time
	delivering              bool
}

// Observe records the counters at time now and reports whether each kind of loss occurred within
// LossRecencyWindow, plus whether the intake is currently delivering. The first call only seeds the
// baseline, reporting no loss and assuming delivery.
//
// A send in the latest interval means delivering; processing or send errors without a send mean not;
// an interval where nothing moved keeps the previous answer, since a stalled pipeline and an idle one look alike.
func (w *LossWindow) Observe(c Counters, now time.Time) (droppedRecently, missedRecently, delivering bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.seeded {
		w.seeded, w.delivering = true, true
		w.lastDropped, w.lastMissed = c.Dropped, c.Missed
		w.lastProcessed, w.lastSent, w.lastErrors = c.Processed, c.Sent, c.Errors
		return false, false, true
	}
	if c.Dropped > w.lastDropped {
		w.droppedAt = now
	}
	if c.Missed > w.lastMissed {
		w.missedAt = now
	}
	switch {
	case c.Sent > w.lastSent:
		w.delivering = true
	case c.Processed > w.lastProcessed || c.Errors > w.lastErrors:
		w.delivering = false
	}
	w.lastDropped, w.lastMissed = c.Dropped, c.Missed
	w.lastProcessed, w.lastSent, w.lastErrors = c.Processed, c.Sent, c.Errors
	droppedRecently = !w.droppedAt.IsZero() && now.Sub(w.droppedAt) <= LossRecencyWindow
	missedRecently = !w.missedAt.IsZero() && now.Sub(w.missedAt) <= LossRecencyWindow
	return droppedRecently, missedRecently, w.delivering
}
