// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// BaselineConfig controls detector-specific baseline qualification windows.
// DurationSec is the qualification duration after a detector's own warmup.
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

// baselineReferenceInterval translates point-count detector requirements into
// data-time warmups. It is deliberately a scheduling contract, not a claim
// about the cadence of every incoming series.
const baselineReferenceInterval = 15 * time.Second

// detectorBaselineSpecProvider is intentionally private: baseline policy is
// engine orchestration, not part of the public detector API.
type detectorBaselineSpecProvider interface {
	BaselineSpec() detectorBaselineSpec
}

type detectorBaselineSpec struct {
	Participate    bool
	WarmupDuration time.Duration
}

type detectorBaselineState struct {
	spec               detectorBaselineSpec
	warmupEndSec       int64
	baselineEndSec     int64
	completed          bool
	windowAnomalyCount int
	pendingHashes      map[uint64]struct{}
}

// BaselineDetectorDebugStatus is a testbench-facing snapshot of one detector.
type BaselineDetectorDebugStatus struct {
	Name           string `json:"name"`
	WarmupEndSec   int64  `json:"warmupEndSec"`
	BaselineEndSec int64  `json:"baselineEndSec"`
	Completed      bool   `json:"completed"`
	MutedCount     int    `json:"mutedCount"`
}

// BaselineDebugStatus is a testbench-facing snapshot of the baseline union.
type BaselineDebugStatus struct {
	Started     bool                          `json:"started"`
	StartSec    int64                         `json:"startSec"`
	AllComplete bool                          `json:"allComplete"`
	MutedCount  int                           `json:"mutedCount"`
	Detectors   []BaselineDetectorDebugStatus `json:"detectors"`
}

// baselineController coordinates independent detector windows. All methods
// run on the engine goroutine.
type baselineController struct {
	config      BaselineConfig
	startSec    int64
	started     bool
	detectors   map[string]*detectorBaselineState
	mutedHashes map[uint64]struct{} // immutable snapshot is published at each completion
	mutedNames  map[string]struct{} // retained for the final verbose summary
}

func newBaselineController(cfg BaselineConfig, detectors []detectorBaselineSpecEntry) *baselineController {
	b := &baselineController{config: cfg, detectors: make(map[string]*detectorBaselineState), mutedHashes: make(map[uint64]struct{}), mutedNames: make(map[string]struct{})}
	for _, d := range detectors {
		if !d.spec.Participate {
			continue
		}
		b.detectors[d.name] = &detectorBaselineState{spec: d.spec, pendingHashes: make(map[uint64]struct{})}
	}
	return b
}

type detectorBaselineSpecEntry struct {
	name string
	spec detectorBaselineSpec
}

func baselineSpecs(detectors []observerdef.Detector) []detectorBaselineSpecEntry {
	entries := make([]detectorBaselineSpecEntry, 0, len(detectors))
	for _, detector := range detectors {
		spec := detectorBaselineSpec{Participate: true}
		if provider, ok := detector.(detectorBaselineSpecProvider); ok {
			spec = provider.BaselineSpec()
		}
		entries = append(entries, detectorBaselineSpecEntry{name: detector.Name(), spec: spec})
	}
	return entries
}

// start seeds all detector windows from the first analysis data timestamp.
func (b *baselineController) start(dataSec int64) {
	if b.started {
		return
	}
	b.started = true
	b.startSec = dataSec
	for _, state := range b.detectors {
		state.warmupEndSec = dataSec + int64(state.spec.WarmupDuration/time.Second)
		state.baselineEndSec = state.warmupEndSec + b.config.DurationSec
	}
}

// isAnalyzingAt reports whether the detector's baseline decision is still in
// progress. During analysis, anomalies are suppressed.
func (b *baselineController) isAnalyzingAt(name string, dataSec int64) bool {
	state := b.detectors[name]
	if state == nil || state.completed {
		return false
	}
	return dataSec < state.baselineEndSec
}

// isQualifyingAt reports whether anomalies should contribute to this
// detector's muted-series decision. The earlier warmup interval is analysis
// only: anomalies are suppressed but never qualify a series for muting.
func (b *baselineController) isQualifyingAt(name string, dataSec int64) bool {
	state := b.detectors[name]
	return state != nil && !state.completed && dataSec >= state.warmupEndSec && dataSec < state.baselineEndSec
}

func (b *baselineController) mark(name string, h uint64) {
	state := b.detectors[name]
	if state == nil || state.completed {
		return
	}
	state.windowAnomalyCount++
	state.pendingHashes[h] = struct{}{}
}

func (b *baselineController) due(dataSec int64) []string {
	var names []string
	for name, state := range b.detectors {
		if !state.completed && dataSec >= state.baselineEndSec {
			names = append(names, name)
		}
	}
	return names
}

func (b *baselineController) complete(name string) (newHashes map[uint64]struct{}, anomalyCount int, allComplete bool) {
	state := b.detectors[name]
	if state == nil || state.completed {
		return nil, 0, b.allComplete()
	}
	state.completed = true
	newHashes = make(map[uint64]struct{})
	for h := range state.pendingHashes {
		if _, exists := b.mutedHashes[h]; !exists {
			b.mutedHashes[h] = struct{}{}
			newHashes[h] = struct{}{}
		}
	}
	return newHashes, state.windowAnomalyCount, b.allComplete()
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
		status.Detectors = append(status.Detectors, BaselineDetectorDebugStatus{Name: name, WarmupEndSec: state.warmupEndSec, BaselineEndSec: state.baselineEndSec, Completed: state.completed, MutedCount: len(state.pendingHashes)})
	}
	sort.Slice(status.Detectors, func(i, j int) bool { return status.Detectors[i].Name < status.Detectors[j].Name })
	return status
}

func (b *baselineController) recordMutedNames(names []string) {
	for _, name := range names {
		b.mutedNames[name] = struct{}{}
	}
}

func (b *baselineController) mutedDisplayNames() []string {
	names := make([]string, 0, len(b.mutedNames))
	for name := range b.mutedNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func cloneMutedHashes(in map[uint64]struct{}) map[uint64]struct{} {
	out := make(map[uint64]struct{}, len(in))
	for h := range in {
		out[h] = struct{}{}
	}
	return out
}
