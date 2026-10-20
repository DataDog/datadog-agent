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

// SenderLatencyHighThresholdMs is the intake latency above which a send bottleneck is latency-bound.
const SenderLatencyHighThresholdMs = 250

// LossRecencyWindow is how long a loss counts as recent.
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

// ComponentRank orders components upstream to downstream; destinations and unknown ones come last.
func ComponentRank(name string) int {
	if r, ok := componentSortOrder[name]; ok {
		return r
	}
	return 10
}

// IsSendStage reports whether the component is a worker, the sender, or a destination.
func IsSendStage(component string) bool {
	return component == "worker" || component == logsmetrics.SenderTlmName || strings.HasPrefix(component, "destination_")
}

// Bottleneck returns the deepest currently saturated stage, else the deepest recently saturated one.
func Bottleneck(stages []Stage) string {
	if c := mostDownstreamSaturated(stages, func(s Stage) bool { return s.CurrentlySaturated }); c != "" {
		return c
	}
	return mostDownstreamSaturated(stages, func(s Stage) bool {
		return s.Saturated1mSeconds > 0 || s.Saturated30mSeconds > 0
	})
}

// mostDownstreamSaturated returns the deepest stage for which sat is true; backpressure propagates upstream.
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

// ForBottleneck maps a bottleneck to a profile, reason code and diagnosis.
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

// Recommend suggests a profile when bytes were recently missed, or returns nil.
func Recommend(stages []Stage, activeProfile string, s Signals) *Recommendation {
	if !s.MissedRecently {
		return nil
	}

	bottleneck := Bottleneck(stages)
	if bottleneck == "" || (IsSendStage(bottleneck) && !s.Delivering) {
		return nil
	}

	profile, code, reason := ForBottleneck(bottleneck, s.SenderLatencyMs)
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

// ReadCounters reads logsmetrics.LogsExpvars-shaped counters; a nil map yields zeros.
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

// LossWindow turns the monotonic counters into recent-loss and delivery signals.
type LossWindow struct {
	mu                      sync.Mutex
	seeded                  bool
	lastMissed              int64
	lastProcessed, lastSent int64
	lastErrors              int64
	missedAt                time.Time
	delivering              bool
}

// Observe reports recent missed bytes and whether the intake is delivering; the first call seeds the baseline.
func (w *LossWindow) Observe(c Counters, now time.Time) (missedRecently, delivering bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.seeded {
		w.seeded, w.delivering = true, true
		w.lastMissed = c.Missed
		w.lastProcessed, w.lastSent, w.lastErrors = c.Processed, c.Sent, c.Errors
		return false, true
	}
	if c.Missed > w.lastMissed {
		w.missedAt = now
	}
	// An idle interval keeps the previous answer: stalled and idle look alike.
	switch {
	case c.Sent > w.lastSent:
		w.delivering = true
	case c.Processed > w.lastProcessed || c.Errors > w.lastErrors:
		w.delivering = false
	}
	w.lastMissed = c.Missed
	w.lastProcessed, w.lastSent, w.lastErrors = c.Processed, c.Sent, c.Errors
	missedRecently = !w.missedAt.IsZero() && now.Sub(w.missedAt) <= LossRecencyWindow
	return missedRecently, w.delivering
}

// SampleInterval is how often a Sampler observes the counters.
const SampleInterval = 15 * time.Second

// Sampler observes the counters on a fixed interval so reading the signals does not change them.
type Sampler struct {
	read   func() Counters
	window LossWindow

	mu                 sync.RWMutex
	missed, delivering bool

	ticker   *time.Ticker
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

func newSampler(read func() Counters, now time.Time) *Sampler {
	s := &Sampler{read: read, delivering: true, stop: make(chan struct{}), done: make(chan struct{})}
	s.window.Observe(read(), now)
	return s
}

// StartSampler observes read every SampleInterval until Stop.
func StartSampler(read func() Counters) *Sampler {
	s := newSampler(read, time.Now())
	s.ticker = time.NewTicker(SampleInterval)
	go func() {
		defer close(s.done)
		for {
			select {
			case now := <-s.ticker.C:
				s.observe(now)
			case <-s.stop:
				return
			}
		}
	}()
	return s
}

func (s *Sampler) observe(now time.Time) {
	missed, delivering := s.window.Observe(s.read(), now)
	s.mu.Lock()
	s.missed, s.delivering = missed, delivering
	s.mu.Unlock()
}

// Signals returns the latest observation; a nil Sampler reports no loss and delivery.
func (s *Sampler) Signals() (missedRecently, delivering bool) {
	if s == nil {
		return false, true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.missed, s.delivering
}

// Stop ends sampling; it is safe on a nil or stopped Sampler.
func (s *Sampler) Stop() {
	if s == nil || s.ticker == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.ticker.Stop()
		close(s.stop)
		<-s.done
	})
}
