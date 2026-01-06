// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"strings"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSustainedElevationDetector_Name(t *testing.T) {
	detector := NewSustainedElevationDetector()
	assert.Equal(t, "sustained_elevation_detector", detector.Name())
}

func TestSustainedElevationDetector_ReturnsEmptyForTooFewPoints(t *testing.T) {
	detector := NewSustainedElevationDetector()

	// Test with 0 points
	result := detector.Analyze(observer.Series{
		Name:   "test.metric",
		Points: []observer.Point{},
	})
	assert.Empty(t, result.Anomalies, "should return empty for 0 points")

	// Test with 4 points (less than default MinPoints=5)
	result = detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 10},
			{Timestamp: 2, Value: 20},
			{Timestamp: 3, Value: 30},
			{Timestamp: 4, Value: 40},
		},
	})
	assert.Empty(t, result.Anomalies, "should return empty for 4 points when MinPoints=5")

	// Test with custom MinPoints
	detector.MinPoints = 10
	result = detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 10},
			{Timestamp: 2, Value: 20},
			{Timestamp: 3, Value: 30},
			{Timestamp: 4, Value: 40},
			{Timestamp: 5, Value: 50},
			{Timestamp: 6, Value: 60},
			{Timestamp: 7, Value: 70},
			{Timestamp: 8, Value: 80},
			{Timestamp: 9, Value: 90},
		},
	})
	assert.Empty(t, result.Anomalies, "should return empty for 9 points when MinPoints=10")
}

func TestSustainedElevationDetector_ReturnsEmptyWhenWithinThreshold(t *testing.T) {
	detector := NewSustainedElevationDetector()

	// Baseline: [10, 10, 10], Recent: [11, 11, 11]
	// Baseline mean = 10, stddev = 0 -> skip detection (constant baseline)
	result := detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 10},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 10},
			{Timestamp: 4, Value: 11},
			{Timestamp: 5, Value: 11},
			{Timestamp: 6, Value: 11},
		},
	})
	assert.Empty(t, result.Anomalies, "should return empty when baseline has zero stddev")

	// Baseline with variance, recent mean within threshold
	// Baseline: [8, 10, 12], mean = 10, stddev = 2
	// Recent: [11, 12, 13], mean = 12
	// Threshold = baseline_mean + 2*stddev = 10 + 4 = 14
	// Recent mean (12) < 14, so no anomaly
	result = detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 11},
			{Timestamp: 5, Value: 12},
			{Timestamp: 6, Value: 13},
		},
	})
	assert.Empty(t, result.Anomalies, "should return empty when recent mean is within threshold")
}

func TestSustainedElevationDetector_ReturnsAnomalyWhenExceedsThreshold(t *testing.T) {
	detector := NewSustainedElevationDetector()

	// Baseline: [8, 10, 12], mean = 10, stddev = 2
	// Recent: [16, 17, 18], mean = 17
	// Threshold = baseline_mean + 2*stddev = 10 + 4 = 14
	// Recent mean (17) > 14, so anomaly
	result := detector.Analyze(observer.Series{
		Namespace: "test.ns",
		Name:      "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 16},
			{Timestamp: 5, Value: 17},
			{Timestamp: 6, Value: 18},
		},
	})

	require.Len(t, result.Anomalies, 1, "should return one anomaly")
	anomaly := result.Anomalies[0]
	assert.Equal(t, "Sustained elevation: test.metric", anomaly.Title)
}

func TestSustainedElevationDetector_AnomalyDescriptionContainsValues(t *testing.T) {
	detector := NewSustainedElevationDetector()

	// Baseline: [8, 10, 12], mean = 10, stddev = 2
	// Recent: [16, 17, 18], mean = 17
	result := detector.Analyze(observer.Series{
		Name: "cpu.usage",
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 16},
			{Timestamp: 5, Value: 17},
			{Timestamp: 6, Value: 18},
		},
	})

	require.Len(t, result.Anomalies, 1)
	desc := result.Anomalies[0].Description

	// Check that description contains metric name
	assert.True(t, strings.Contains(desc, "cpu.usage"), "description should contain metric name")

	// Check that description contains recent avg (17.00)
	assert.True(t, strings.Contains(desc, "17.00"), "description should contain recent avg")

	// Check that description contains baseline (10.00)
	assert.True(t, strings.Contains(desc, "10.00"), "description should contain baseline avg")

	// Check that description contains threshold (2.00 stddev)
	assert.True(t, strings.Contains(desc, "2.00"), "description should contain threshold stddev")
}

func TestSustainedElevationDetector_CopiesTagsFromSeries(t *testing.T) {
	detector := NewSustainedElevationDetector()

	tags := []string{"env:prod", "service:api", "region:us-east-1"}
	result := detector.Analyze(observer.Series{
		Name: "test.metric",
		Tags: tags,
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 100},
			{Timestamp: 5, Value: 100},
			{Timestamp: 6, Value: 100},
		},
	})

	require.Len(t, result.Anomalies, 1)
	assert.Equal(t, tags, result.Anomalies[0].Tags, "anomaly tags should match series tags")
}

func TestSustainedElevationDetector_SourceIsSetToSeriesName(t *testing.T) {
	detector := NewSustainedElevationDetector()

	result := detector.Analyze(observer.Series{
		Name:      "memory.used",
		Namespace: "system",
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 100},
			{Timestamp: 5, Value: 100},
			{Timestamp: 6, Value: 100},
		},
	})

	require.Len(t, result.Anomalies, 1)
	assert.Equal(t, "memory.used", result.Anomalies[0].Source, "source should be series name")
}

func TestSustainedElevationDetector_CustomThreshold(t *testing.T) {
	detector := &SustainedElevationDetector{
		MinPoints: 6,
		Threshold: 1.0, // More sensitive threshold
	}

	// Baseline: [8, 10, 12], mean = 10, stddev = 2
	// Recent: [13, 14, 15], mean = 14
	// Threshold = baseline_mean + 1*stddev = 10 + 2 = 12
	// Recent mean (14) > 12, so anomaly with threshold=1.0
	result := detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 13},
			{Timestamp: 5, Value: 14},
			{Timestamp: 6, Value: 15},
		},
	})

	require.Len(t, result.Anomalies, 1, "should detect anomaly with lower threshold")

	// Same data but with default threshold=2.0 should NOT detect
	detector.Threshold = 2.0
	result = detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 13},
			{Timestamp: 5, Value: 14},
			{Timestamp: 6, Value: 15},
		},
	})
	assert.Empty(t, result.Anomalies, "should not detect anomaly with higher threshold")
}

func TestSustainedElevationDetector_DefaultsAppliedWhenZero(t *testing.T) {
	// Create detector with zero values (should use defaults)
	detector := &SustainedElevationDetector{
		MinPoints: 0,
		Threshold: 0,
	}

	// Should use default MinPoints=5, so 4 points should be rejected
	result := detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 10},
			{Timestamp: 2, Value: 20},
			{Timestamp: 3, Value: 30},
			{Timestamp: 4, Value: 40},
		},
	})
	assert.Empty(t, result.Anomalies, "should use default MinPoints when set to 0")

	// Should use default Threshold=2.0
	// With 6 points, baseline [8,10,12] mean=10, stddev=2
	// Recent [13,14,15] mean=14, threshold=10+2*2=14
	// 14 is NOT > 14, so no anomaly
	result = detector.Analyze(observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1, Value: 8},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 12},
			{Timestamp: 4, Value: 13},
			{Timestamp: 5, Value: 14},
			{Timestamp: 6, Value: 15},
		},
	})
	assert.Empty(t, result.Anomalies, "should use default Threshold when set to 0")
}

func TestMean(t *testing.T) {
	tests := []struct {
		name     string
		points   []observer.Point
		expected float64
	}{
		{
			name:     "empty",
			points:   []observer.Point{},
			expected: 0,
		},
		{
			name:     "single value",
			points:   []observer.Point{{Value: 5}},
			expected: 5,
		},
		{
			name:     "multiple values",
			points:   []observer.Point{{Value: 2}, {Value: 4}, {Value: 6}},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mean(tt.points)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSampleStddev(t *testing.T) {
	tests := []struct {
		name     string
		points   []observer.Point
		mean     float64
		expected float64
	}{
		{
			name:     "empty",
			points:   []observer.Point{},
			mean:     0,
			expected: 0,
		},
		{
			name:     "single value",
			points:   []observer.Point{{Value: 5}},
			mean:     5,
			expected: 0,
		},
		{
			name: "values [8, 10, 12] with mean 10",
			points: []observer.Point{
				{Value: 8},
				{Value: 10},
				{Value: 12},
			},
			mean:     10,
			expected: 2, // sqrt((4+0+4)/2) = sqrt(4) = 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sampleStddev(tt.points, tt.mean)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}
