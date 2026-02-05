// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
)

func TestRobustZScoreDetector_Name(t *testing.T) {
	d := NewRobustZScoreDetector()
	assert.Equal(t, "robust_zscore", d.Name())
}

func TestRobustZScoreDetector_NotEnoughPoints(t *testing.T) {
	d := NewRobustZScoreDetector()
	series := observer.Series{
		Name:   "test.metric",
		Points: []observer.Point{{Timestamp: 1, Value: 10}},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestRobustZScoreDetector_StableData(t *testing.T) {
	d := NewRobustZScoreDetector()

	// Generate stable data around 100
	points := make([]observer.Point, 20)
	for i := 0; i < 20; i++ {
		points[i] = observer.Point{Timestamp: int64(i), Value: 100 + float64(i%3-1)}
	}

	series := observer.Series{Name: "test.metric", Points: points}
	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies, "stable data should not trigger anomalies")
}

func TestRobustZScoreDetector_DetectsUpwardSpike(t *testing.T) {
	d := NewRobustZScoreDetector()

	// Baseline data followed by a spike
	points := make([]observer.Point, 20)
	for i := 0; i < 15; i++ {
		points[i] = observer.Point{Timestamp: int64(i), Value: 100}
	}
	// Add a significant spike
	for i := 15; i < 20; i++ {
		points[i] = observer.Point{Timestamp: int64(i), Value: 500}
	}

	series := observer.Series{Name: "test.metric", Points: points}
	result := d.Analyze(series)

	assert.Len(t, result.Anomalies, 1)
	assert.Contains(t, result.Anomalies[0].Description, "above")
}

func TestRobustZScoreDetector_DetectsDownwardSpike(t *testing.T) {
	d := NewRobustZScoreDetector()

	// Baseline data followed by a drop
	points := make([]observer.Point, 20)
	for i := 0; i < 15; i++ {
		points[i] = observer.Point{Timestamp: int64(i), Value: 100}
	}
	// Add a significant drop
	for i := 15; i < 20; i++ {
		points[i] = observer.Point{Timestamp: int64(i), Value: 10}
	}

	series := observer.Series{Name: "test.metric", Points: points}
	result := d.Analyze(series)

	assert.Len(t, result.Anomalies, 1)
	assert.Contains(t, result.Anomalies[0].Description, "below")
}

func TestMedian(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{"empty", []float64{}, 0},
		{"single", []float64{5}, 5},
		{"odd count", []float64{1, 3, 5}, 3},
		{"even count", []float64{1, 2, 3, 4}, 2.5},
		{"unsorted", []float64{5, 1, 3, 2, 4}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := median(tt.values)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMAD(t *testing.T) {
	// Values: 1, 2, 3, 4, 5
	// Median: 3
	// Deviations from median: |1-3|=2, |2-3|=1, |3-3|=0, |4-3|=1, |5-3|=2
	// MAD = median(2, 1, 0, 1, 2) = median(0, 1, 1, 2, 2) = 1
	values := []float64{1, 2, 3, 4, 5}
	med := median(values)
	result := mad(values, med)
	assert.Equal(t, 1.0, result)
}
