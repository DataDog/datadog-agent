// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
)

const (
	// saturationHighThreshold is the fill percentage above which a stage is considered saturated.
	saturationHighThreshold = 0.70
	// saturationLowThreshold is the fill percentage below which a saturated stage is considered recovered.
	// The gap between high and low provides hysteresis to prevent rapid state flapping.
	saturationLowThreshold = 0.30
	// saturationMinDuration is the minimum sustained saturation duration before an event is recorded.
	saturationMinDuration = 10 * time.Second
	// maxSaturationEvents is the maximum number of events retained in the ring buffer.
	maxSaturationEvents = 50

	// retryRateWindow is the sliding window over which transport retries are counted.
	retryRateWindow = 30 * time.Second
	// retryRateThreshold is the number of retries within retryRateWindow that marks transport as saturated.
	retryRateThreshold = 3
)

// SaturationEvent records a single episode where a pipeline stage was saturated.
type SaturationEvent struct {
	// Stage is the pipeline stage name (ProcessorTlmName, StrategyTlmName, SenderTlmName).
	Stage string
	// StartTime is when saturation began.
	StartTime time.Time
	// EndTime is when saturation ended; zero means the event is still ongoing.
	EndTime time.Time
	// PeakFill is the highest fill percentage observed during the event (0.0–1.0).
	// For the sender stage this is a normalised retry rate, not a channel fill.
	PeakFill float64
	// Suggestion is the profile name recommended to address this bottleneck.
	Suggestion string
}

// Duration returns how long the event lasted, or the elapsed time if still ongoing.
func (e SaturationEvent) Duration() time.Duration {
	if e.EndTime.IsZero() {
		return time.Since(e.StartTime)
	}
	return e.EndTime.Sub(e.StartTime)
}

// Ongoing reports whether the saturation event is still active.
func (e SaturationEvent) Ongoing() bool { return e.EndTime.IsZero() }

// SaturationSummary is a point-in-time snapshot returned by SaturationHistory.Summary.
type SaturationSummary struct {
	// CurrentFill is the most recently recorded fill percentage per stage (0.0–1.0).
	// Updated on every CapacityMonitor sample tick (approximately once per second).
	CurrentFill map[string]float64
	// MaxFill5m / MaxFill30m / MaxFill2h are the per-stage maximum fill percentages
	// observed in the last 5 minutes, 30 minutes, and 2 hours respectively.
	// Keys are the stage TlmName constants.
	MaxFill5m  map[string]float64
	MaxFill30m map[string]float64
	MaxFill2h  map[string]float64
	// SuggestedProfile is the profile name recommended based on the 5-minute window.
	// Empty string means no recommendation.
	SuggestedProfile string
	// RecentEvents contains the most recent saturation events, newest first.
	RecentEvents []SaturationEvent
}

// ---------- internal types ----------

type stageSaturationState struct {
	saturated bool
	startTime time.Time
	peakFill  float64
}

type rollingWindow struct {
	duration  time.Duration
	maxFill   map[string]float64
	lastReset time.Time
}

func newRollingWindow(d time.Duration, now time.Time) rollingWindow {
	return rollingWindow{
		duration:  d,
		maxFill:   make(map[string]float64),
		lastReset: now,
	}
}

func (w *rollingWindow) record(stage string, fill float64, now time.Time) {
	if now.Sub(w.lastReset) >= w.duration {
		w.maxFill = make(map[string]float64)
		w.lastReset = now
	}
	if fill > w.maxFill[stage] {
		w.maxFill[stage] = fill
	}
}

func (w *rollingWindow) max(stage string) float64 {
	return w.maxFill[stage]
}

// ---------- SaturationHistory ----------

// SaturationHistory tracks historical saturation events across the logs pipeline.
// It is driven by CapacityMonitor sample ticks (for processor / strategy fill %)
// and by RecordRetry calls from the HTTP destination (for transport pressure).
// Access to the singleton is via GlobalSaturationHistory.
type SaturationHistory struct {
	mu      sync.Mutex
	states  map[string]*stageSaturationState // keyed by stage TlmName
	events  []SaturationEvent                // ring buffer, oldest first
	windows [3]rollingWindow                 // indices: 0=5m, 1=30m, 2=2h

	// currentFill is the most recently sampled fill value per stage.
	currentFill map[string]float64

	// retryTimestamps is a sliding window of recent retry event times.
	retryTimestamps []time.Time

	// currentProfile is the logs_agent_profile value set at startup (or via RC).
	currentProfile string

	// now is injectable for testing.
	now func() time.Time
}

// GlobalSaturationHistory is the package-level singleton.
// CapacityMonitor and the HTTP destination write to it; the status builder reads from it.
var GlobalSaturationHistory = NewSaturationHistory()

// SetCurrentProfile stores the active profile name so that recommendation telemetry
// can be tagged with the current setting. Call this at agent startup and on RC updates.
func SetCurrentProfile(name string) {
	GlobalSaturationHistory.mu.Lock()
	GlobalSaturationHistory.currentProfile = name
	GlobalSaturationHistory.mu.Unlock()
}

// NewSaturationHistory creates a new SaturationHistory.
func NewSaturationHistory() *SaturationHistory {
	now := time.Now()
	h := &SaturationHistory{
		states:          make(map[string]*stageSaturationState),
		events:          make([]SaturationEvent, 0, maxSaturationEvents),
		currentFill:     make(map[string]float64),
		retryTimestamps: make([]time.Time, 0, 64),
		now:             time.Now,
	}
	h.windows[0] = newRollingWindow(5*time.Minute, now)
	h.windows[1] = newRollingWindow(30*time.Minute, now)
	h.windows[2] = newRollingWindow(2*time.Hour, now)
	return h
}

// RecordFill records the current fill percentage for a pipeline stage.
// Called by CapacityMonitor on each sample tick when channel capacity is known.
// fill must be in [0.0, 1.0].
func (h *SaturationHistory) RecordFill(stage string, fill float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.currentFill[stage] = fill

	now := h.now()
	for i := range h.windows {
		h.windows[i].record(stage, fill, now)
	}
	h.updateState(stage, fill, now)
	h.emitTelemetry()
}

// RecordRetry records a transport-layer retry event.
// Called by the HTTP destination on every retry attempt.
//
// Retry count is recorded in the rolling windows and currentFill for display
// on the status page, but intentionally does NOT drive the saturation state
// machine or profile recommendations. Retry count measures HTTP errors, not
// backpressure — a few retries is normal and should not trigger a config
// suggestion. Transport saturation detection will be wired to sender worker
// utilization ratio once that signal is available in SaturationHistory.
func (h *SaturationHistory) RecordRetry() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := h.now()

	// Prune timestamps outside the sliding window.
	cutoff := now.Add(-retryRateWindow)
	n := 0
	for _, t := range h.retryTimestamps {
		if t.After(cutoff) {
			h.retryTimestamps[n] = t
			n++
		}
	}
	h.retryTimestamps = append(h.retryTimestamps[:n], now)

	// Normalise retry rate to [0, 1] for display purposes only.
	rate := float64(len(h.retryTimestamps))
	fillEquiv := rate / float64(retryRateThreshold+1)
	if fillEquiv > 1.0 {
		fillEquiv = 1.0
	}
	h.currentFill[SenderTlmName] = fillEquiv

	for i := range h.windows {
		h.windows[i].record(SenderTlmName, fillEquiv, now)
	}
	// Deliberately no updateState or emitTelemetry — see comment above.
}

// updateState runs the per-stage saturation state machine and appends events on recovery.
// Must be called with h.mu held.
func (h *SaturationHistory) updateState(stage string, fill float64, now time.Time) {
	state, ok := h.states[stage]
	if !ok {
		state = &stageSaturationState{}
		h.states[stage] = state
	}

	if !state.saturated && fill >= saturationHighThreshold {
		state.saturated = true
		state.startTime = now
		state.peakFill = fill
	} else if state.saturated {
		if fill > state.peakFill {
			state.peakFill = fill
		}
		if fill < saturationLowThreshold {
			// Recovered. Record the event only if it lasted long enough to be meaningful.
			if now.Sub(state.startTime) >= saturationMinDuration {
				h.appendEvent(SaturationEvent{
					Stage:      stage,
					StartTime:  state.startTime,
					EndTime:    now,
					PeakFill:   state.peakFill,
					Suggestion: suggestionForStage(stage),
				})
			}
			state.saturated = false
			state.peakFill = 0
		}
	}
}

func (h *SaturationHistory) appendEvent(e SaturationEvent) {
	if len(h.events) >= maxSaturationEvents {
		copy(h.events, h.events[1:])
		h.events = h.events[:len(h.events)-1]
	}
	h.events = append(h.events, e)
}

// emitTelemetry fires stage-saturation and profile-recommendation metrics.
// Only stages driven by CapacityMonitor (processor, strategy) participate in
// the saturation state machine; the sender stage is excluded until a reliable
// utilization-based signal replaces the retry-count proxy.
// Must be called with h.mu held.
func (h *SaturationHistory) emitTelemetry() {
	for _, stage := range []string{ProcessorTlmName, StrategyTlmName} {
		v := 0.0
		if s, ok := h.states[stage]; ok && s.saturated {
			v = 1.0
		}
		TlmStageSaturation.Set(v, stage)
	}

	suggestion := h.computeSuggestion()
	currentProfile := h.currentProfile
	if currentProfile == "" {
		currentProfile = "balanced"
	}
	if suggestion != "" && suggestion != currentProfile {
		TlmProfileRecommendationActive.Set(1.0, currentProfile, suggestion)
	} else {
		TlmProfileRecommendationActive.Set(0.0, currentProfile, "none")
	}
}

// computeSuggestion returns the recommended profile based on the 30-minute rolling max.
//
// Using the 30-minute window means a profile change is only suggested when a
// bottleneck has been sustained long enough to warrant a permanent configuration
// change — brief spikes don't count. Transport (sender) is intentionally excluded
// until a reliable non-retry-based signal (sender worker utilization ratio) is wired
// into SaturationHistory.
// Must be called with h.mu held.
func (h *SaturationHistory) computeSuggestion() string {
	strategyFill := h.windows[1].max(StrategyTlmName)
	processorFill := h.windows[1].max(ProcessorTlmName)

	switch {
	case strategyFill >= saturationHighThreshold:
		return "max_throughput"
	case processorFill >= saturationHighThreshold:
		return "performance"
	default:
		return ""
	}
}

// Summary returns a snapshot of the current saturation state for the status page.
func (h *SaturationHistory) Summary() SaturationSummary {
	h.mu.Lock()
	defer h.mu.Unlock()

	stages := []string{ProcessorTlmName, StrategyTlmName, SenderTlmName}
	s := SaturationSummary{
		CurrentFill: make(map[string]float64, len(stages)),
		MaxFill5m:   make(map[string]float64, len(stages)),
		MaxFill30m:  make(map[string]float64, len(stages)),
		MaxFill2h:   make(map[string]float64, len(stages)),
	}
	for _, stage := range stages {
		s.CurrentFill[stage] = h.currentFill[stage]
		s.MaxFill5m[stage] = h.windows[0].max(stage)
		s.MaxFill30m[stage] = h.windows[1].max(stage)
		s.MaxFill2h[stage] = h.windows[2].max(stage)
	}
	s.SuggestedProfile = h.computeSuggestion()

	// Return events newest-first.
	s.RecentEvents = make([]SaturationEvent, len(h.events))
	for i, e := range h.events {
		s.RecentEvents[len(h.events)-1-i] = e
	}
	return s
}

// suggestionForStage returns the profile name that addresses a bottleneck at the given stage.
func suggestionForStage(stage string) string {
	switch stage {
	case ProcessorTlmName:
		return "performance"
	case StrategyTlmName:
		return "max_throughput"
	case SenderTlmName:
		return "wan_optimized"
	default:
		return ""
	}
}

func init() {
	// Initialise all stage saturation gauges to 0 so they exist in Prometheus
	// even before any events are recorded.
	TlmStageSaturation.Set(0.0, ProcessorTlmName)
	TlmStageSaturation.Set(0.0, StrategyTlmName)
	TlmStageSaturation.Set(0.0, SenderTlmName)
}
