// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Baseline analysis suppresses false positives from metrics that are inherently
// noisy at startup.
//
//	t=0                      t=window_end
//	 │◄── baseline window ──►  │
//	 │                         │
//	 │  detectors run normally │  freeze: noisy series identified
//	 │  anomalies → mark only  │   ├─ storage + detector state reclaimed
//	 │  (not forwarded)        │   └─ hash added to filter mute set
//	 │                         │
//	 ▼                         ▼──────────────────────────────────►
//	                              muted series dropped at ingest

package observerimpl

// BaselineConfig controls the baseline analysis window.
type BaselineConfig struct {
	Enabled          bool
	DurationSec      int64
	MuteNoisyMetrics bool
	Verbose          bool // log each muted series name when the window closes
}

// DefaultBaselineConfig returns the default baseline config (enabled, 10m window, muting on).
func DefaultBaselineConfig() BaselineConfig {
	return BaselineConfig{
		Enabled:          true,
		DurationSec:      600,
		MuteNoisyMetrics: true,
	}
}

// baselineController manages the baseline muting window. All methods run
// exclusively from the engine run goroutine — no locking required.
type baselineController struct {
	config             BaselineConfig
	startSec           int64
	windowAnomalyCount int
	frozen             bool
	// mutedHashes accumulates noisy series during the window and persists as the
	// mute set afterwards. Keyed by seriesKeyHash(namespace, name, tags).
	mutedHashes map[uint64]struct{}
}

func newBaselineController(cfg BaselineConfig) *baselineController {
	return &baselineController{
		config:      cfg,
		mutedHashes: make(map[uint64]struct{}),
	}
}

// activeAt reports whether the window is still open at dataSec, seeding
// startSec on the first call.
func (b *baselineController) activeAt(dataSec int64) bool {
	if b.startSec == 0 {
		b.startSec = dataSec
	}
	return dataSec < b.startSec+b.config.DurationSec
}

// mark records a series as noisy by its hash. No-op when frozen.
// Caller must ensure tags are sorted so the hash matches storage.
func (b *baselineController) mark(h uint64) {
	if b.frozen {
		return
	}
	b.windowAnomalyCount++
	b.mutedHashes[h] = struct{}{}
}

// shouldFreeze reports whether the window should close at dataSec.
func (b *baselineController) shouldFreeze(dataSec int64) bool {
	return !b.frozen && b.startSec > 0 && !b.activeAt(dataSec)
}

// freeze closes the window. mutedHashes is already populated by mark calls.
// Called at most once, guarded by shouldFreeze.
func (b *baselineController) freeze() int {
	b.frozen = true
	return b.windowAnomalyCount
}

// reset clears all state for a fresh replay.
func (b *baselineController) reset() {
	b.startSec = 0
	b.mutedHashes = make(map[uint64]struct{})
	b.windowAnomalyCount = 0
	b.frozen = false
}
