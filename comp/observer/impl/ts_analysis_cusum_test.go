// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build observer

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCUSUMDetector_Name(t *testing.T) {
	d := NewCUSUMDetector()
	assert.Equal(t, "cusum_detector", d.Name())
}

func TestCUSUMDetector_NotEnoughPoints(t *testing.T) {
	d := NewCUSUMDetector()

	series := observer.Series{
		Namespace: "test",
		Name:      "metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 10},
			{Timestamp: 2, Value: 10},
		},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies, "should not detect with too few points")
}

func TestCUSUMDetector_StableData(t *testing.T) {
	d := NewCUSUMDetector()

	// 20 points of stable data around 10 with small noise
	points := make([]observer.Point, 20)
	for i := range points {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     10.0 + float64(i%3-1)*0.5, // 9.5, 10, 10.5 pattern
		}
	}

	series := observer.Series{
		Namespace: "test",
		Name:      "stable_metric",
		Points:    points,
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies, "should not detect anomaly in stable data")
}

func TestCUSUMDetector_DetectsShift(t *testing.T) {
	d := NewCUSUMDetector()

	// Build a series: baseline around 10, then shifts to 50
	points := make([]observer.Point, 20)
	for i := 0; i < 10; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     10.0 + float64(i%3-1)*0.5, // baseline: ~10
		}
	}
	for i := 10; i < 20; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     50.0 + float64(i%3-1)*0.5, // shifted: ~50
		}
	}

	series := observer.Series{
		Namespace: "test",
		Name:      "shifted_metric",
		Points:    points,
	}

	result := d.Analyze(series)
	require.Len(t, result.Anomalies, 1, "should detect the shift")

	anomaly := result.Anomalies[0]
	assert.Equal(t, "shifted_metric", anomaly.Source)
	assert.Contains(t, anomaly.Title, "CUSUM")
	assert.Contains(t, anomaly.Description, "shifted")

	// Timestamp should be around when the threshold was first crossed (shortly after point 10)
	assert.GreaterOrEqual(t, anomaly.Timestamp, int64(10), "detection should be at or after shift point")
	assert.LessOrEqual(t, anomaly.Timestamp, int64(15), "detection should be near shift point")
}

func TestCUSUMDetector_GradualIncrease(t *testing.T) {
	d := NewCUSUMDetector()

	// Baseline, then gradual increase
	points := make([]observer.Point, 30)
	for i := 0; i < 10; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     10.0,
		}
	}
	// Gradual increase from 10 to 50
	for i := 10; i < 30; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     10.0 + float64(i-10)*2.0, // 10, 12, 14, ... 48
		}
	}

	series := observer.Series{
		Namespace: "test",
		Name:      "gradual_metric",
		Points:    points,
	}

	result := d.Analyze(series)
	require.Len(t, result.Anomalies, 1, "should detect gradual increase")

	// The anomaly timestamp should be somewhere after baseline period
	assert.Greater(t, result.Anomalies[0].Timestamp, int64(5))
}

func TestCUSUMDetector_ConstantBaseline(t *testing.T) {
	d := NewCUSUMDetector()

	// Constant baseline of 1, then jumps to 3
	points := make([]observer.Point, 20)
	for i := 0; i < 10; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     1.0, // constant baseline
		}
	}
	for i := 10; i < 20; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     3.0, // 3x increase
		}
	}

	series := observer.Series{
		Namespace: "test",
		Name:      "constant_baseline",
		Points:    points,
	}

	result := d.Analyze(series)
	require.Len(t, result.Anomalies, 1, "should detect shift even with constant baseline")
}

func TestCUSUMDetector_CustomParameters(t *testing.T) {
	// More sensitive detector
	d := &CUSUMDetector{
		MinPoints:        3,
		BaselineFraction: 0.5,
		SlackFactor:      0.25, // less slack = more sensitive
		ThresholdFactor:  2.0,  // lower threshold = triggers earlier
	}

	// Small shift that default detector might miss
	points := make([]observer.Point, 10)
	for i := 0; i < 5; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     10.0,
		}
	}
	for i := 5; i < 10; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     15.0, // 50% increase
		}
	}

	series := observer.Series{
		Namespace: "test",
		Name:      "small_shift",
		Points:    points,
	}

	result := d.Analyze(series)
	assert.Len(t, result.Anomalies, 1, "sensitive detector should catch small shift")
}

func TestCUSUMDetector_SourceAndTags(t *testing.T) {
	d := NewCUSUMDetector()

	points := make([]observer.Point, 20)
	for i := 0; i < 10; i++ {
		points[i] = observer.Point{Timestamp: int64(i), Value: 10.0}
	}
	for i := 10; i < 20; i++ {
		points[i] = observer.Point{Timestamp: int64(i), Value: 100.0}
	}

	series := observer.Series{
		Namespace: "demo",
		Name:      "my.metric:avg",
		Tags:      []string{"env:prod", "service:api"},
		Points:    points,
	}

	result := d.Analyze(series)
	require.Len(t, result.Anomalies, 1)

	anomaly := result.Anomalies[0]
	assert.Equal(t, "my.metric:avg", anomaly.Source, "Source should match series name")
	assert.Equal(t, []string{"env:prod", "service:api"}, anomaly.Tags, "Tags should be preserved")
}

func TestCUSUMDetector_EmitsAtThresholdCrossing(t *testing.T) {
	d := NewCUSUMDetector()

	// Build a series: baseline (10), then elevated (50), then back to baseline (10)
	// With the new point-based approach, we should emit at the first threshold crossing
	points := make([]observer.Point, 30)
	for i := 0; i < 10; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     10.0, // baseline
		}
	}
	for i := 10; i < 20; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     50.0, // elevated
		}
	}
	for i := 20; i < 30; i++ {
		points[i] = observer.Point{
			Timestamp: int64(i),
			Value:     10.0, // back to baseline
		}
	}

	series := observer.Series{
		Namespace: "test",
		Name:      "recovers",
		Points:    points,
	}

	result := d.Analyze(series)
	require.Len(t, result.Anomalies, 1, "should detect the anomaly")

	anomaly := result.Anomalies[0]

	// Timestamp should be at or shortly after point 10 (when elevation began)
	// This is when the threshold is first crossed, not the entire duration
	assert.GreaterOrEqual(t, anomaly.Timestamp, int64(10), "detection should be at or after elevation start")
	assert.LessOrEqual(t, anomaly.Timestamp, int64(15), "detection should be near elevation start")
}
