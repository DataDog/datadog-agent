// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// relativeChangeGateName identifies the relative-change gate in telemetry.
const relativeChangeGateName = "relative_change"

const defaultStableMinRelativeChange = 0.05

// StableScanMWConfig exposes the stable ScanMW tuning surface.
type StableScanMWConfig struct {
	MinPoints int `json:"min_points"`
	MaxPoints int `json:"max_points"`
}

// StableScanWelchConfig exposes the stable ScanWelch tuning surface.
type StableScanWelchConfig struct {
	MinPoints int `json:"min_points"`
	MaxPoints int `json:"max_points"`
}

// StableBOCPDConfig exposes the stable BOCPD tuning surface.
type StableBOCPDConfig struct {
	WarmupPoints int `json:"warmup_points"`
	MaxRunLength int `json:"max_run_length"`
}

// StablePipelineConfig holds construction-time stable pipeline parameters.
type StablePipelineConfig struct {
	// MinRelativeChange is the minimum relative change between the before and
	// after medians required to emit an anomaly. Zero disables the gate.
	MinRelativeChange float64 `json:"min_relative_change"`
	// ScanMW selects ScanMW when non-nil. An empty block uses defaults.
	ScanMW *StableScanMWConfig `json:"scanmw,omitempty"`
	// ScanWelch selects ScanWelch when non-nil. An empty block uses defaults.
	ScanWelch *StableScanWelchConfig `json:"scanwelch,omitempty"`
	// BOCPD selects BOCPD when non-nil. An empty block uses defaults.
	BOCPD *StableBOCPDConfig `json:"bocpd,omitempty"`
}

// DefaultStablePipelineConfig gates stable detection at a 5% minimum change.
func DefaultStablePipelineConfig() StablePipelineConfig {
	return StablePipelineConfig{
		MinRelativeChange: defaultStableMinRelativeChange,
	}
}

// validateStableDetectorSelection enforces the single-detector rule.
func validateStableDetectorSelection(cfg StablePipelineConfig) error {
	selected := 0
	for _, present := range []bool{cfg.ScanMW != nil, cfg.ScanWelch != nil, cfg.BOCPD != nil} {
		if present {
			selected++
		}
	}
	if selected > 1 {
		return errors.New("stable pipeline accepts at most one detector block")
	}
	return nil
}

// ParseStablePipelineConfigJSON decodes stable pipeline JSON configuration.
func ParseStablePipelineConfigJSON(raw []byte) (*StablePipelineConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	stable := DefaultStablePipelineConfig()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stable); err != nil {
		return nil, fmt.Errorf("parse stable pipeline config: %w", err)
	}
	if err := validateStableDetectorSelection(stable); err != nil {
		return nil, err
	}
	return &stable, nil
}

// newStableDetector selects the detector configured by stable-pipeline blocks.
func newStableDetector(cfg StablePipelineConfig) (stableDetector, error) {
	if err := validateStableDetectorSelection(cfg); err != nil {
		return nil, err
	}
	switch {
	case cfg.ScanMW != nil:
		return NewScanMWDetector(ScanMWConfig{
			MinPoints: cfg.ScanMW.MinPoints,
			MaxPoints: cfg.ScanMW.MaxPoints,
		}), nil
	case cfg.ScanWelch != nil:
		return NewScanWelchDetector(ScanWelchConfig{
			MinPoints: cfg.ScanWelch.MinPoints,
			MaxPoints: cfg.ScanWelch.MaxPoints,
		}), nil
	case cfg.BOCPD != nil:
		return newBOCPDDetector(BOCPDConfig{
			WarmupPoints: cfg.BOCPD.WarmupPoints,
			MaxRunLength: cfg.BOCPD.MaxRunLength,
		}, true), nil
	default:
		return NewScanMWDetector(DefaultScanMWConfig()), nil
	}
}

// stableDetector is the detector contract StablePipeline composes.
type stableDetector interface {
	pipelineDetector
	observer.DetectorPointWindowRequirement
}

// StablePipeline runs a stable-series detector and mutes candidates whose
// level change is below the relative threshold. Muting never affects the
// detector's own state.
type StablePipeline struct {
	detector              stableDetector
	gate                  *relativeChangeGate // nil when the gate is disabled
	warnedMissingEvidence bool
}

// newStablePipeline wraps detector with the configured gate. A threshold of
// zero constructs no gate.
func newStablePipeline(detector stableDetector, cfg StablePipelineConfig) (*StablePipeline, error) {
	if detector == nil {
		return nil, errors.New("stable pipeline requires a detector")
	}
	threshold := cfg.MinRelativeChange
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
		return nil, fmt.Errorf("stable pipeline min_relative_change must be a non-negative finite number, got %v", threshold)
	}
	var gate *relativeChangeGate
	if threshold > 0 {
		gate = &relativeChangeGate{minChange: threshold}
	}
	return &StablePipeline{detector: detector, gate: gate}, nil
}

// Name identifies the composed workflow and its detector, e.g.
// "stable_pipeline:scanmw".
func (p *StablePipeline) Name() string {
	return "stable_pipeline:" + p.detector.Name()
}

// Ready reports the detector readiness, independent of gating outcomes.
func (p *StablePipeline) Ready() bool { return p.detector.Ready() }

// DetectorPointWindow forwards the detector history requirement.
func (p *StablePipeline) DetectorPointWindow() observer.DetectorPointWindow {
	return p.detector.DetectorPointWindow()
}

// Detect forwards detection, then applies the gate to returned candidates.
func (p *StablePipeline) Detect(
	storage observer.StorageReader,
	dataTime int64,
	filter observer.DetectionFilter,
) observer.DetectionResult {
	result := p.detector.Detect(storage, dataTime, filter)
	if p.gate == nil || len(result.Anomalies) == 0 {
		return result
	}
	accepted := make([]observer.Anomaly, 0, len(result.Anomalies))
	for _, anomaly := range result.Anomalies {
		if anomaly.LevelChange == nil && !p.warnedMissingEvidence {
			p.warnedMissingEvidence = true
			logging.Warnf("stable pipeline %s: anomaly without level-change evidence; the gate mutes such candidates", p.Name())
		}
		if p.gate.passes(anomaly) {
			accepted = append(accepted, anomaly)
		} else {
			p.gate.rejected++
		}
	}
	return observer.DetectionResult{Anomalies: accepted}
}

// RemoveSeries forwards series eviction to the detector.
func (p *StablePipeline) RemoveSeries(refs []observer.SeriesRef) {
	p.detector.RemoveSeries(refs)
}

// Reset clears detector state and pending rejection counts.
func (p *StablePipeline) Reset() {
	p.detector.Reset()
	if p.gate != nil {
		p.gate.clear()
	}
}

// drainGatedRejections returns and clears the number of candidates muted by
// the gate since the last drain. Zero when the gate is disabled.
func (p *StablePipeline) drainGatedRejections() uint64 {
	if p.gate == nil {
		return 0
	}
	return p.gate.drain()
}

// relativeChangeGate mutes anomalies whose before/after medians differ by less
// than the configured relative threshold. It owns only rejection counters.
type relativeChangeGate struct {
	minChange float64
	rejected  uint64
}

// passes reports whether the anomaly satisfies the relative change requirement.
func (g *relativeChangeGate) passes(anomaly observer.Anomaly) bool {
	if anomaly.LevelChange == nil {
		return false
	}
	return meetsRelativeChange(
		anomaly.LevelChange.BeforeMedian,
		anomaly.LevelChange.AfterMedian,
		g.minChange,
	)
}

// drain returns and clears pending rejection counts.
func (g *relativeChangeGate) drain() uint64 {
	count := g.rejected
	g.rejected = 0
	return count
}

// clear discards pending rejection counts.
func (g *relativeChangeGate) clear() { g.rejected = 0 }

// meetsRelativeChange reports whether the level change from before to after is
// at least minChange. Equality passes and direction is irrelevant. Relative
// change is measured against the magnitude of before, so a sign flip always
// passes. When before is zero, any nonzero after passes. Non-finite values fail.
func meetsRelativeChange(before, after, minChange float64) bool {
	if minChange <= 0 {
		return true
	}
	if math.IsNaN(before) || math.IsNaN(after) || math.IsInf(before, 0) || math.IsInf(after, 0) {
		return false
	}
	if before == 0 {
		return after != 0
	}
	return math.Abs(after-before) >= minChange*math.Abs(before)
}
