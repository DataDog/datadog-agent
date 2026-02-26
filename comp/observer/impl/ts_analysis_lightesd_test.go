// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLightESD_Name(t *testing.T) {
	emitter := NewLightESDEmitter(DefaultLightESDConfig())
	assert.Equal(t, "lightesd", emitter.Name())
}

func TestLightESD_NotEnoughPoints(t *testing.T) {
	emitter := NewLightESDEmitter(DefaultLightESDConfig())

	series := observer.Series{
		Name: "test.metric",
		Points: []observer.Point{
			{Timestamp: 1000, Value: 10.0},
			{Timestamp: 1001, Value: 11.0},
		},
	}

	result := emitter.Analyze(series)
	assert.Empty(t, result.Anomalies, "should not detect anomalies with insufficient points")
}

func TestLightESD_StableData(t *testing.T) {
	emitter := NewLightESDEmitter(DefaultLightESDConfig())

	// Generate stable data around 100.0
	var points []observer.Point
	for i := 0; i < 100; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0,
		})
	}

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	assert.Empty(t, result.Anomalies, "should not detect anomalies in stable data")
}

func TestLightESD_DetectsSingleOutlier(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.MaxOutliers = 5
	config.EnablePeriodicity = false // Disable for simpler test
	emitter := NewLightESDEmitter(config)

	// Generate baseline: normal data around 100.0
	var points []observer.Point
	for i := 0; i < 50; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0 + float64(i%5)*0.1, // Small variation
		})
	}

	// Insert a clear outlier
	points[25].Value = 200.0

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
		Tags:   []string{"env:test"},
	}

	result := emitter.Analyze(series)
	require.GreaterOrEqual(t, len(result.Anomalies), 1, "should detect at least one anomaly")

	// Verify the outlier was detected
	found := false
	for _, a := range result.Anomalies {
		if a.Timestamp == 1025 && a.DebugInfo != nil && a.DebugInfo.CurrentValue == 200.0 {
			found = true
			assert.Equal(t, observer.MetricName("test.metric"), a.Source)
			assert.Equal(t, []string{"env:test"}, a.Tags)
			assert.Greater(t, a.DebugInfo.DeviationSigma, 0.0)
		}
	}
	assert.True(t, found, "should detect the planted outlier at timestamp 1025")
}

func TestLightESD_DetectsMultipleOutliers(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.MaxOutliers = 10
	config.EnablePeriodicity = false
	emitter := NewLightESDEmitter(config)

	// Generate baseline
	var points []observer.Point
	for i := 0; i < 100; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0,
		})
	}

	// Insert multiple EXTREME outliers (robust methods need clear deviations)
	points[20].Value = 500.0  // Very extreme
	points[50].Value = -200.0 // Very extreme
	points[80].Value = 600.0  // Very extreme

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	assert.GreaterOrEqual(t, len(result.Anomalies), 2, "should detect multiple outliers")
}

func TestLightESD_WithTrend(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.EnablePeriodicity = false
	emitter := NewLightESDEmitter(config)

	// Generate data with upward trend
	var points []observer.Point
	for i := 0; i < 100; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0 + float64(i)*0.5, // Linear trend
		})
	}

	// Insert EXTREME outlier that deviates from trend
	points[50].Value = 500.0 // Much higher above trend line

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	require.GreaterOrEqual(t, len(result.Anomalies), 1, "should detect outlier despite trend")
}

func TestLightESD_WithSeasonality(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.EnablePeriodicity = true // Enable seasonal decomposition
	emitter := NewLightESDEmitter(config)

	// Generate data with clear seasonality (period = 10)
	period := 10
	var points []observer.Point
	for i := 0; i < 100; i++ {
		// Seasonal pattern: sine wave
		seasonal := 10.0 * math.Sin(2*math.Pi*float64(i)/float64(period))
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0 + seasonal,
		})
	}

	// Insert outlier that breaks seasonal pattern
	points[55].Value = 150.0

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	require.GreaterOrEqual(t, len(result.Anomalies), 1, "should detect outlier in seasonal data")
}

func TestLightESD_IgnoresSmallDeviations(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.Alpha = 0.05 // Standard significance level
	config.EnablePeriodicity = false
	emitter := NewLightESDEmitter(config)

	// Generate data with natural variation
	var points []observer.Point
	for i := 0; i < 100; i++ {
		// Add small random-like variation
		variation := float64((i*7)%10) - 5.0 // Deterministic "noise"
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0 + variation,
		})
	}

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	assert.Empty(t, result.Anomalies, "should not detect anomalies in data with natural variation")
}

func TestLightESD_CustomAlpha(t *testing.T) {
	// More strict (less sensitive)
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.Alpha = 0.01 // More strict = fewer false positives
	config.EnablePeriodicity = false
	emitter := NewLightESDEmitter(config)

	var points []observer.Point
	for i := 0; i < 100; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0,
		})
	}

	// Moderate outlier
	points[50].Value = 120.0

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	// With stricter alpha, moderate outlier might not be detected
	t.Logf("Detected %d anomalies with alpha=0.01", len(result.Anomalies))
}

func TestLightESD_MaxOutliersLimit(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.MaxOutliers = 2 // Limit to 2 outliers
	config.EnablePeriodicity = false
	emitter := NewLightESDEmitter(config)

	var points []observer.Point
	for i := 0; i < 100; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0,
		})
	}

	// Insert 5 outliers
	points[20].Value = 200.0
	points[40].Value = 200.0
	points[60].Value = 200.0
	points[70].Value = 200.0
	points[80].Value = 200.0

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	assert.LessOrEqual(t, len(result.Anomalies), config.MaxOutliers,
		"should not exceed MaxOutliers limit")
}

func TestLightESD_TagsPropagated(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.EnablePeriodicity = false
	emitter := NewLightESDEmitter(config)

	var points []observer.Point
	for i := 0; i < 60; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0,
		})
	}
	points[30].Value = 500.0 // Extreme outlier

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
		Tags:   []string{"env:prod", "service:api", "host:web-01"},
	}

	result := emitter.Analyze(series)
	require.GreaterOrEqual(t, len(result.Anomalies), 1)

	for _, a := range result.Anomalies {
		assert.Equal(t, []string{"env:prod", "service:api", "host:web-01"}, a.Tags)
	}
}

func TestLightESD_ScoreReflectsSeverity(t *testing.T) {
	config := DefaultLightESDConfig()
	config.MinWindowSize = 50
	config.EnablePeriodicity = false
	emitter := NewLightESDEmitter(config)

	var points []observer.Point
	for i := 0; i < 60; i++ {
		points = append(points, observer.Point{
			Timestamp: int64(1000 + i),
			Value:     100.0,
		})
	}

	// Two outliers of different magnitudes (both extreme for robust detection)
	points[25].Value = 300.0 // Moderate extreme
	points[40].Value = 600.0 // Severe extreme

	series := observer.Series{
		Name:   "test.metric",
		Points: points,
	}

	result := emitter.Analyze(series)
	require.GreaterOrEqual(t, len(result.Anomalies), 2)

	// Find scores for each outlier
	var moderateScore, severeScore float64
	for _, a := range result.Anomalies {
		if a.DebugInfo != nil && a.DebugInfo.CurrentValue == 300.0 {
			moderateScore = a.DebugInfo.DeviationSigma
		} else if a.DebugInfo != nil && a.DebugInfo.CurrentValue == 600.0 {
			severeScore = a.DebugInfo.DeviationSigma
		}
	}

	assert.Greater(t, severeScore, moderateScore,
		"more severe outlier should have higher score")
}

// Test helper functions

func TestComputeMedian(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	median := computeMedian(data)
	assert.Equal(t, 3.0, median)

	dataEven := []float64{1, 2, 3, 4}
	medianEven := computeMedian(dataEven)
	assert.Equal(t, 2.5, medianEven)
}

func TestMedianAbsoluteDeviation(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	mad := medianAbsoluteDeviation(data)
	// MAD of [1,2,3,4,5] with median=3 is median([2,1,0,1,2]) = 1
	assert.Equal(t, 1.0, mad)
}

func TestExtractRobustTrend(t *testing.T) {
	// Data with linear trend
	var data []float64
	for i := 0; i < 50; i++ {
		data = append(data, 100.0+float64(i)*2.0)
	}

	trend := extractRobustTrend(data, 5)
	require.Len(t, trend, 50)

	// Trend should be smooth and roughly follow the linear pattern
	assert.InDelta(t, 100.0, trend[0], 5.0)
	assert.InDelta(t, 198.0, trend[49], 5.0)
}

func TestRobustGeneralizedESD_NoOutliers(t *testing.T) {
	// Normal data
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	outliers := robustGeneralizedESD(data, 3, 0.05)

	assert.Empty(t, outliers, "should not detect outliers in normal data")
}

func TestRobustGeneralizedESD_WithOutliers(t *testing.T) {
	// Data with clear outliers
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 100, 200}
	outliers := robustGeneralizedESD(data, 5, 0.05)

	assert.GreaterOrEqual(t, len(outliers), 2, "should detect the two outliers")

	// Check that indices 9 and 10 (values 100, 200) are detected
	hasNine := false
	hasTen := false
	for _, idx := range outliers {
		if idx == 9 {
			hasNine = true
		}
		if idx == 10 {
			hasTen = true
		}
	}
	assert.True(t, hasNine || hasTen, "should detect at least one of the planted outliers")
}
