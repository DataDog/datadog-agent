// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stableStubDetector struct {
	name      string
	ready     bool
	window    observer.DetectorPointWindow
	anomalies []observer.Anomaly
	calls     int
	removed   []observer.SeriesRef
	resets    int
}

func (d *stableStubDetector) Name() string { return d.name }
func (d *stableStubDetector) Ready() bool  { return d.ready }

func (d *stableStubDetector) Detect(
	_ observer.StorageReader,
	_ int64,
	_ observer.DetectionFilter,
) observer.DetectionResult {
	d.calls++
	return observer.DetectionResult{Anomalies: d.anomalies}
}

func (d *stableStubDetector) RemoveSeries(refs []observer.SeriesRef) {
	d.removed = append(d.removed, refs...)
}

func (d *stableStubDetector) Reset() { d.resets++ }

func (d *stableStubDetector) DetectorPointWindow() observer.DetectorPointWindow {
	return d.window
}

func stableEvidenceAnomaly(before, after float64) observer.Anomaly {
	return observer.Anomaly{
		Timestamp:   1,
		LevelChange: &observer.LevelChangeEvidence{BeforeMedian: before, AfterMedian: after},
	}
}

func TestNewStablePipeline_Config(t *testing.T) {
	_, err := newStablePipeline(nil, DefaultStablePipelineConfig())
	require.Error(t, err)

	_, err = newStablePipeline(&stableStubDetector{}, StablePipelineConfig{MinRelativeChange: -0.1})
	require.Error(t, err)
	_, err = newStablePipeline(&stableStubDetector{}, StablePipelineConfig{MinRelativeChange: math.NaN()})
	require.Error(t, err)
	_, err = newStablePipeline(&stableStubDetector{}, StablePipelineConfig{MinRelativeChange: math.Inf(1)})
	require.Error(t, err)

	p, err := newStablePipeline(&stableStubDetector{name: "scanmw"}, StablePipelineConfig{MinRelativeChange: 0})
	require.NoError(t, err)
	assert.Nil(t, p.gate, "zero threshold must construct no gate")

	p, err = newStablePipeline(&stableStubDetector{name: "scanmw"}, DefaultStablePipelineConfig())
	require.NoError(t, err)
	require.NotNil(t, p.gate)
	assert.InDelta(t, 0.05, p.gate.minChange, 1e-12)
}

func TestStablePipeline_PassthroughWhenDisabled(t *testing.T) {
	detector := &stableStubDetector{name: "scanmw", ready: true, anomalies: []observer.Anomaly{
		{Timestamp: 1},
		{Timestamp: 2, LevelChange: &observer.LevelChangeEvidence{BeforeMedian: 100, AfterMedian: 101}},
	}}
	p, err := newStablePipeline(detector, StablePipelineConfig{MinRelativeChange: 0})
	require.NoError(t, err)

	result := p.Detect(nil, 2, observer.DetectionFilter{})
	assert.Len(t, result.Anomalies, 2, "disabled gate must pass everything")
	assert.Equal(t, 1, detector.calls)
	assert.Zero(t, p.drainGatedRejections())
}

func TestStablePipeline_GateThresholds(t *testing.T) {
	tests := []struct {
		name    string
		before  float64
		after   float64
		minimum float64
		pass    bool
	}{
		{name: "below threshold", before: 100, after: 104, minimum: 0.05, pass: false},
		{name: "at threshold", before: 100, after: 105, minimum: 0.05, pass: true},
		{name: "above threshold", before: 100, after: 106, minimum: 0.05, pass: true},
		{name: "downward", before: 100, after: 94, minimum: 0.05, pass: true},
		{name: "negative levels", before: -100, after: -104, minimum: 0.05, pass: false},
		{name: "negative decrease", before: -100, after: -90, minimum: 0.05, pass: true},
		{name: "negative to positive", before: -100, after: 10, minimum: 0.05, pass: true},
		{name: "zero to nonzero", before: 0, after: 1, minimum: 0.05, pass: true},
		{name: "zero to zero", before: 0, after: 0, minimum: 0.05, pass: false},
		{name: "nan baseline", before: math.NaN(), after: 100, minimum: 0.05, pass: false},
		{name: "inf after", before: 100, after: math.Inf(1), minimum: 0.05, pass: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.pass, meetsRelativeChange(tc.before, tc.after, tc.minimum))
		})
	}
}

func TestStablePipeline_DetectFiltersAndCountsRejections(t *testing.T) {
	detector := &stableStubDetector{name: "scanmw", ready: true, anomalies: []observer.Anomaly{
		stableEvidenceAnomaly(100, 106),
		stableEvidenceAnomaly(100, 103), // below threshold
		{Timestamp: 3},                  // missing evidence
	}}
	p, err := newStablePipeline(detector, DefaultStablePipelineConfig())
	require.NoError(t, err)

	result := p.Detect(nil, 3, observer.DetectionFilter{})
	require.Len(t, result.Anomalies, 1)
	assert.Equal(t, int64(1), result.Anomalies[0].Timestamp)
	assert.Equal(t, 1, detector.calls, "detector must run for every advance")

	assert.Equal(t, uint64(2), p.drainGatedRejections())
	assert.Zero(t, p.drainGatedRejections(), "drain must clear counts")
}

func TestStablePipeline_LifecycleDelegation(t *testing.T) {
	detector := &stableStubDetector{
		name:   "scanmw",
		ready:  true,
		window: observer.DetectorPointWindow{MinPoints: 30, MaxPoints: 120},
	}
	p, err := newStablePipeline(detector, DefaultStablePipelineConfig())
	require.NoError(t, err)

	assert.Equal(t, "stable_pipeline:scanmw", p.Name())
	assert.True(t, p.Ready())
	assert.Equal(t, detector.window, p.DetectorPointWindow())

	p.RemoveSeries([]observer.SeriesRef{7})
	assert.Equal(t, []observer.SeriesRef{7}, detector.removed)

	// Pending rejections are cleared by reset.
	p.gate.rejected = 3
	p.Reset()
	assert.Equal(t, 1, detector.resets)
	assert.Zero(t, p.drainGatedRejections())
}

func TestStablePipeline_ReadyDespiteRejections(t *testing.T) {
	detector := &stableStubDetector{name: "scanmw", ready: true, anomalies: []observer.Anomaly{
		stableEvidenceAnomaly(100, 101),
	}}
	p, err := newStablePipeline(detector, DefaultStablePipelineConfig())
	require.NoError(t, err)

	assert.Empty(t, p.Detect(nil, 1, observer.DetectionFilter{}).Anomalies)
	assert.True(t, p.Ready(), "readiness must not depend on gating outcomes")
}

func TestStablePipeline_MutedChangeAdvancesDetectorState(t *testing.T) {
	cfg := testScanMWPipelineConfig()
	buildStorage := func() *timeSeriesStorage {
		s := newTimeSeriesStorage()
		for i := int64(1); i <= 20; i++ {
			s.Add("ns", "metric", 100, i, nil)
		}
		for i := int64(21); i <= 40; i++ {
			s.Add("ns", "metric", 103, i, nil)
		}
		return s
	}
	gated, err := newStablePipeline(NewScanMWDetector(cfg), DefaultStablePipelineConfig())
	require.NoError(t, err)
	ungated, err := newStablePipeline(NewScanMWDetector(cfg), StablePipelineConfig{})
	require.NoError(t, err)

	gatedResult := gated.Detect(buildStorage(), 40, observer.DetectionFilter{})
	ungatedResult := ungated.Detect(buildStorage(), 40, observer.DetectionFilter{})

	require.Empty(t, gatedResult.Anomalies, "3%% change must be muted")
	require.NotEmpty(t, ungatedResult.Anomalies)
	require.Equal(t, ungated.detector, gated.detector,
		"muting must not alter ScanMW state progression")
}

func TestStablePipeline_WarnsOnceForMissingEvidence(t *testing.T) {
	detector := &stableStubDetector{name: "scanmw", ready: true, anomalies: []observer.Anomaly{
		{Timestamp: 1},
		{Timestamp: 2},
	}}
	p, err := newStablePipeline(detector, DefaultStablePipelineConfig())
	require.NoError(t, err)

	result := p.Detect(nil, 2, observer.DetectionFilter{})
	assert.Empty(t, result.Anomalies)
	assert.True(t, p.warnedMissingEvidence, "missing evidence must warn once")

	p.Detect(nil, 2, observer.DetectionFilter{})
	assert.True(t, p.warnedMissingEvidence)
	assert.Equal(t, uint64(4), p.drainGatedRejections())
}
