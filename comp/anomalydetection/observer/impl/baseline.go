// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// BaselineConfig controls detector-specific baseline qualification windows.
// DurationSec is the qualification duration after a detector becomes ready.
type BaselineConfig struct {
	Enabled          bool
	DurationSec      int64
	MuteNoisyMetrics bool
	Verbose          bool // log each muted series when every baseline has completed
}

// DefaultBaselineConfig returns the default baseline config (enabled, 10m qualification, muting on).
func DefaultBaselineConfig() BaselineConfig {
	return BaselineConfig{Enabled: true, DurationSec: 600, MuteNoisyMetrics: true}
}

type detectorBaselineState struct {
	ready              bool
	warmupEndSec       int64
	baselineEndSec     int64
	completed          bool
	windowAnomalyCount int
	pendingHashes      map[uint64]struct{}
	mutedCount         int
}

// BaselineDetectorDebugStatus is a testbench-facing snapshot of one detector.
type BaselineDetectorDebugStatus struct {
	Name           string `json:"name"`
	Ready          bool   `json:"ready"`
	WarmupEndSec   int64  `json:"warmupEndSec,omitempty"`
	BaselineEndSec int64  `json:"baselineEndSec,omitempty"`
	Completed      bool   `json:"completed"`
	MutedCount     int    `json:"mutedCount"`
}

// BaselineDebugStatus is a testbench-facing snapshot of the baseline union.
type BaselineDebugStatus struct {
	Started            bool                          `json:"started"`
	StartSec           int64                         `json:"startSec"`
	AnalyzedThroughSec int64                         `json:"analyzedThroughSec,omitempty"`
	AllComplete        bool                          `json:"allComplete"`
	MutedCount         int                           `json:"mutedCount"`
	Detectors          []BaselineDetectorDebugStatus `json:"detectors"`
}

// baselineController coordinates independent detector windows. All methods
// run on the engine goroutine.
//
//	data time ──►  [ warmup ] [ qualification ] [ detection ]
//	normal detector  model       suppress + mute    forward
//	RRCF             model       suppress           forward
//
// Each detector owns its first two windows and must not emit anomalies during
// warmup. A normal detector's anomalies observed while analysing can mute their
// source series globally; RRCF has no source series and therefore only uses the
// windows to suppress its own reports.
type baselineController struct {
	config    BaselineConfig
	startSec  int64
	started   bool
	detectors map[string]*detectorBaselineState
	// mutedHashes is an immutable snapshot. complete replaces it rather than
	// mutating it, so the same map can safely be published to concurrent ingest
	// handlers and retained by synchronous event consumers.
	mutedHashes map[uint64]struct{}
	mutedNames  map[string]struct{} // allocated only for the final verbose summary
}

func newBaselineController(cfg BaselineConfig, detectorNames []string) *baselineController {
	b := &baselineController{config: cfg, detectors: make(map[string]*detectorBaselineState), mutedHashes: make(map[uint64]struct{})}
	for _, name := range detectorNames {
		b.detectors[name] = &detectorBaselineState{pendingHashes: make(map[uint64]struct{})}
	}
	return b
}

func detectorNames(detectors []observerdef.Detector) []string {
	names := make([]string, 0, len(detectors))
	for _, detector := range detectors {
		names = append(names, detector.Name())
	}
	return names
}

// start records the first analysis timestamp. Individual qualification windows
// begin only when their detector reports that it is ready to score.
func (b *baselineController) start(dataSec int64) {
	if b.started {
		return
	}
	b.started = true
	b.startSec = dataSec
}

// ready records a detector's first usable scoring advance and starts its
// qualification baseline. It returns true only for that first transition.
func (b *baselineController) ready(name string, dataSec int64) bool {
	state := b.detectors[name]
	if state == nil || state.ready {
		return false
	}
	state.ready = true
	state.warmupEndSec = dataSec
	state.baselineEndSec = dataSec + b.config.DurationSec
	return true
}

// isAnalyzingAt reports whether the detector's baseline decision is still in
// progress. During analysis, anomalies are suppressed.
func (b *baselineController) isAnalyzingAt(name string, dataSec int64) bool {
	state := b.detectors[name]
	if state == nil || state.completed {
		return false
	}
	if !state.ready {
		return true
	}
	return dataSec < state.baselineEndSec
}

func (b *baselineController) mark(name string, h uint64) {
	state := b.detectors[name]
	if state == nil || state.completed || !state.ready {
		return
	}
	state.windowAnomalyCount++
	state.pendingHashes[h] = struct{}{}
}

func (b *baselineController) due(dataSec int64) []string {
	var names []string
	for name, state := range b.detectors {
		if state.ready && !state.completed && dataSec >= state.baselineEndSec {
			names = append(names, name)
		}
	}
	return names
}

func (b *baselineController) complete(name string) (newHashes map[uint64]struct{}, snapshotChanged bool, anomalyCount int, allComplete bool) {
	state := b.detectors[name]
	if state == nil || !state.ready || state.completed {
		return nil, false, 0, b.allComplete()
	}
	state.completed = true
	state.mutedCount = len(state.pendingHashes)
	newHashes = state.pendingHashes
	state.pendingHashes = nil // release the per-detector set after its window ends

	// Reuse the detached pending set as the delta. The old union is immutable,
	// so discard duplicates from the private delta before building one new union.
	for h := range newHashes {
		if _, exists := b.mutedHashes[h]; exists {
			delete(newHashes, h)
		}
	}
	if len(newHashes) == 0 {
		return newHashes, false, state.windowAnomalyCount, b.allComplete()
	}

	next := make(map[uint64]struct{}, len(b.mutedHashes)+len(newHashes))
	for h := range b.mutedHashes {
		next[h] = struct{}{}
	}
	for h := range newHashes {
		next[h] = struct{}{}
	}
	b.mutedHashes = next
	return newHashes, true, state.windowAnomalyCount, b.allComplete()
}

func (b *baselineController) allComplete() bool {
	return b.completedCount() == len(b.detectors)
}

func (b *baselineController) completedCount() int {
	count := 0
	for _, state := range b.detectors {
		if state.completed {
			count++
		}
	}
	return count
}

func (b *baselineController) debugStatus() BaselineDebugStatus {
	status := BaselineDebugStatus{Started: b.started, StartSec: b.startSec, AllComplete: b.allComplete(), MutedCount: len(b.mutedHashes)}
	for name, state := range b.detectors {
		mutedCount := len(state.pendingHashes)
		if state.completed {
			mutedCount = state.mutedCount
		}
		status.Detectors = append(status.Detectors, BaselineDetectorDebugStatus{Name: name, Ready: state.ready, WarmupEndSec: state.warmupEndSec, BaselineEndSec: state.baselineEndSec, Completed: state.completed, MutedCount: mutedCount})
	}
	sort.Slice(status.Detectors, func(i, j int) bool { return status.Detectors[i].Name < status.Detectors[j].Name })
	return status
}

func (b *baselineController) recordMutedNames(names []string) {
	if len(names) == 0 {
		return
	}
	if b.mutedNames == nil {
		b.mutedNames = make(map[string]struct{}, len(names))
	}
	for _, name := range names {
		b.mutedNames[name] = struct{}{}
	}
}

// takeMutedDisplayNames returns the final verbose summary and releases the
// retained series-name strings immediately afterwards.
func (b *baselineController) takeMutedDisplayNames() []string {
	names := make([]string, 0, len(b.mutedNames))
	for name := range b.mutedNames {
		names = append(names, name)
	}
	b.mutedNames = nil
	sort.Strings(names)
	return names
}
