// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package detectors

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/assert"
)

// generateContextKey creates a ContextKey for testing purposes
func generateContextKey(name, hostname string, tags []string) ckey.ContextKey {
	keygen := ckey.NewKeyGenerator()
	return keygen.Generate(name, hostname, tagset.NewHashingTagsAccumulatorWithTags(tags))
}

func TestMeanChangeDetector_Name(t *testing.T) {
	detector := NewMeanChangeDetector()
	assert.Equal(t, "mean_change", detector.Name())
}

func TestMeanChangeDetector_DetectsChangepoint(t *testing.T) {
	detector := NewMeanChangeDetector()

	// Create a series with a clear changepoint using sliding window approach
	// With delta-first, we need one extra point to compute deltas
	// Baseline (10 deltas): stable values around 10.0 (delta ≈ 0)
	// Recent window (5 deltas): jumping to 100.0 (large positive deltas)
	key := generateContextKey("test.metric", "", []string{"env:test"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](20),
	}

	// Add baseline: 11 points around 10.0 (creates 10 deltas of ~0)
	for i := 0; i < 11; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   10.0,
				Min:   10.0,
				Max:   10.0,
			},
		})
	}

	// Add recent window: 5 points around 100.0 (creates 5 deltas: first one is +90, rest are ~0)
	for i := 11; i < 16; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   100.0,
				Min:   100.0,
				Max:   100.0,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify anomaly detected
	assert.NotNil(t, anomalies)
	assert.Equal(t, 1, len(anomalies))

	anomaly := anomalies[0]
	assert.Equal(t, "mean_change", anomaly.DetectorName)
	assert.Equal(t, "changepoint", anomaly.Type)
	assert.Equal(t, history.Key, anomaly.SeriesKey)
	assert.Greater(t, anomaly.Severity, 0.0)
	assert.LessOrEqual(t, anomaly.Severity, 1.0)
	assert.Contains(t, anomaly.Message, "increase")
	assert.Contains(t, anomaly.Message, "rate of change")
	// Timestamp should be at the start of the recent window (point 11)
	assert.Equal(t, int64(11*10), anomaly.Timestamp)
}

func TestMeanChangeDetector_DetectsDecrease(t *testing.T) {
	detector := NewMeanChangeDetector()

	// Create a series with a decrease using sliding window approach
	// Baseline (10 deltas): stable values around 100.0 (delta ≈ 0)
	// Recent window (5 deltas): dropping to 10.0 (large negative delta)
	key := generateContextKey("test.metric", "", []string{"env:test"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](20),
	}

	// Add baseline: 11 points around 100.0 (creates 10 deltas of ~0)
	for i := 0; i < 11; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   100.0,
				Min:   100.0,
				Max:   100.0,
			},
		})
	}

	// Add recent window: 5 points around 10.0 (creates 5 deltas: first one is -90, rest are ~0)
	for i := 11; i < 16; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   10.0,
				Min:   10.0,
				Max:   10.0,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify anomaly detected
	assert.NotNil(t, anomalies)
	assert.Equal(t, 1, len(anomalies))

	anomaly := anomalies[0]
	assert.Equal(t, "mean_change", anomaly.DetectorName)
	assert.Equal(t, "changepoint", anomaly.Type)
	assert.Contains(t, anomaly.Message, "decrease")
	assert.Contains(t, anomaly.Message, "rate of change")
}

func TestMeanChangeDetector_NoDetectionWhenStable(t *testing.T) {
	detector := NewMeanChangeDetector()

	// Create a series with stable values throughout baseline and recent window
	// All deltas will be ~0, so no anomaly should be detected
	key := generateContextKey("test.metric", "", []string{"env:test"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](20),
	}

	// Add 16 points all around 50.0 (creates 15 deltas: 10 baseline + 5 recent)
	for i := 0; i < 16; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   50.0,
				Min:   50.0,
				Max:   50.0,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify no anomaly detected
	assert.Nil(t, anomalies)
}

func TestMeanChangeDetector_NoDetectionWithSmallChange(t *testing.T) {
	detector := NewMeanChangeDetector()

	// Create a series with a small change (below threshold)
	// Baseline: values around 100.0 with slight variation (deltas ~1)
	// Recent: values around 101.0 with similar variation (deltas still ~1)
	key := generateContextKey("test.metric", "", []string{"env:test"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](20),
	}

	// Add baseline: 11 points around 100.0 with slight variation
	for i := 0; i < 11; i++ {
		value := 100.0 + float64(i%3) // values: 100, 101, 102, 100, 101...
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Add recent window: 5 points around 101.0 with similar variation
	for i := 11; i < 16; i++ {
		value := 101.0 + float64(i%3)
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify no anomaly detected (change is too small)
	assert.Nil(t, anomalies)
}

func TestMeanChangeDetector_InsufficientData(t *testing.T) {
	detector := NewMeanChangeDetector()

	// Create a series with insufficient data (less than BaselineWindowSize + RecentWindowSize + 1)
	key := generateContextKey("test.metric", "", []string{"env:test"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](20),
	}

	// Add only 15 points (less than default BaselineWindowSize + RecentWindowSize + 1 = 16)
	for i := 0; i < 15; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   100.0,
				Min:   100.0,
				Max:   100.0,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify no anomaly detected (insufficient data)
	assert.Nil(t, anomalies)
}

func TestMeanChangeDetector_CustomThreshold(t *testing.T) {
	// Create detector with higher threshold
	detector := &MeanChangeDetector{
		Threshold:          5.0, // higher threshold
		RecentWindowSize:   5,
		BaselineWindowSize: 10,
	}

	// Create a series with a moderate change that has some variance
	// Baseline deltas will have some variance
	// Recent deltas will have similar variance pattern
	key := generateContextKey("test.metric", "", []string{"env:test"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](20),
	}

	// Add baseline: 11 points around 100.0 with variance (90-110)
	for i := 0; i < 11; i++ {
		value := 100.0 + float64(i%3)*5.0 - 5.0 // values: 95, 100, 105, 95, 100, 105...
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Add recent window: 5 points around 115.0 with similar variance (105-125)
	// The deltas will be similar to baseline deltas (around +5, 0, -5 pattern)
	// This is below our threshold of 5.0 stddev
	for i := 11; i < 16; i++ {
		value := 115.0 + float64(i%3)*5.0 - 5.0 // values: 110, 115, 120, 110, 115...
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify no anomaly detected (change doesn't exceed higher threshold of 5.0)
	assert.Nil(t, anomalies)
}

func TestMeanChangeDetector_SlidingWindowAdvantage(t *testing.T) {
	// This test demonstrates the advantage of sliding window over half-split:
	// A spike in the middle gets averaged into both halves in half-split approach,
	// but sliding window can detect it if it appears in the recent window.
	// With delta-first, we detect the sudden rate change.
	detector := NewMeanChangeDetector()

	key := generateContextKey("test.metric", "", []string{"env:test"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](25),
	}

	// Add baseline: 11 points at steady state around 50.0 (creates 10 deltas of ~0)
	for i := 0; i < 11; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   50.0,
				Min:   50.0,
				Max:   50.0,
			},
		})
	}

	// Add recent window: 5 points with a significant spike to 150.0 (creates large deltas)
	for i := 11; i < 16; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   150.0,
				Min:   150.0,
				Max:   150.0,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify anomaly detected - sliding window approach catches the spike
	assert.NotNil(t, anomalies)
	assert.Equal(t, 1, len(anomalies))
	assert.Contains(t, anomalies[0].Message, "increase")
	assert.Contains(t, anomalies[0].Message, "rate of change")
}

func TestMeanChangeDetector_MonotonicCounterConstantRate(t *testing.T) {
	// Test that monotonic counters with constant rate are NOT flagged
	// Example: system.uptime increasing by ~90 each interval
	detector := NewMeanChangeDetector()

	key := generateContextKey("system.uptime", "", []string{})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "system.uptime",
			Tags:       []string{},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](25),
	}

	// Simulate monotonic counter: uptime starting at 5270000, increasing by 90 each interval
	baseValue := 5270000.0
	increment := 90.0
	for i := 0; i < 16; i++ {
		value := baseValue + float64(i)*increment
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify no anomaly detected - constant rate produces low variance in deltas
	assert.Nil(t, anomalies, "Monotonic counter with constant rate should not be flagged")
}

func TestMeanChangeDetector_MonotonicCounterRateChange(t *testing.T) {
	// Test that monotonic counters with rate changes ARE flagged
	// Example: uptime suddenly increasing faster
	detector := NewMeanChangeDetector()

	key := generateContextKey("system.uptime", "", []string{})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "system.uptime",
			Tags:       []string{},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](25),
	}

	// Baseline: uptime increasing by 90 each interval
	baseValue := 5270000.0
	increment := 90.0
	for i := 0; i < 11; i++ {
		value := baseValue + float64(i)*increment
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Recent window: uptime suddenly increasing by 500 each interval (rate change!)
	newIncrement := 500.0
	for i := 11; i < 16; i++ {
		value := baseValue + 11.0*increment + float64(i-11)*newIncrement
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify anomaly detected - rate change is significant
	assert.NotNil(t, anomalies, "Counter with rate change should be flagged")
	assert.Equal(t, 1, len(anomalies))
	assert.Contains(t, anomalies[0].Message, "increase")
	assert.Contains(t, anomalies[0].Message, "rate of change")
}

func TestMeanChangeDetector_GaugeWithSustainedIncrease(t *testing.T) {
	// Test that gauges with sustained level increases ARE flagged
	// Example: load average jumping from 2.0 to 20.0 and staying high
	// The delta-first approach detects the sudden rate of increase
	detector := NewMeanChangeDetector()

	key := generateContextKey("system.load.1", "", []string{})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "system.load.1",
			Tags:       []string{},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](25),
	}

	// Baseline: stable load around 2.0 (creates deltas of ~0)
	for i := 0; i < 11; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   2.0,
				Min:   2.0,
				Max:   2.0,
			},
		})
	}

	// Recent window: load jumps to 20.0 and stays there
	// This creates a large positive delta followed by deltas of ~0
	// Recent deltas: [+18, 0, 0, 0, 0] mean = +3.6
	// Baseline deltas: [0, 0, ...] mean = 0, std = 0 (we use 1 as fallback)
	// Change: |3.6 - 0| / 1 = 3.6 > 2.0 threshold -> DETECTED
	for i := 11; i < 16; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   20.0,
				Min:   20.0,
				Max:   20.0,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify anomaly detected - sustained increase shows up as non-zero mean delta
	assert.NotNil(t, anomalies, "Gauge with sustained increase should be flagged")
	assert.Equal(t, 1, len(anomalies))
	assert.Contains(t, anomalies[0].Message, "increase")
	assert.Contains(t, anomalies[0].Message, "rate of change")
}

func TestMeanChangeDetector_NegligibleChangeFiltering(t *testing.T) {
	// Test that metrics with negligible changes (like system.disk.utilized: 57.97 -> 57.97)
	// are NOT flagged even if the stddev is very low.
	// This tests the filtering added to avoid false positives from near-constant metrics.
	detector := NewMeanChangeDetector()

	key := generateContextKey("system.disk.utilized", "", []string{"device:disk0"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "system.disk.utilized",
			Tags:       []string{"device:disk0"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](25),
	}

	// Simulate a nearly constant metric with tiny floating point variations
	// Values: 57.970001, 57.970002, 57.970001, ... (variations in the 6th decimal)
	for i := 0; i < 16; i++ {
		value := 57.97 + float64(i%3)*0.000001
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify no anomaly detected - changes are negligible
	assert.Nil(t, anomalies, "Nearly constant metric should not be flagged")
}

func TestMeanChangeDetector_ZeroValueMetric(t *testing.T) {
	// Test that metrics stuck at or near zero (like system.fs.inodes.in_use: 0.00 -> 0.00)
	// are NOT flagged.
	detector := NewMeanChangeDetector()

	key := generateContextKey("system.fs.inodes.in_use", "", []string{"device:disk0"})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "system.fs.inodes.in_use",
			Tags:       []string{"device:disk0"},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](25),
	}

	// Simulate a metric hovering around zero with tiny variations
	for i := 0; i < 16; i++ {
		value := 0.0 + float64(i%3)*0.0000001
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 10),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify no anomaly detected - metric is essentially zero
	assert.Nil(t, anomalies, "Near-zero metric with tiny variations should not be flagged")
}

func TestMeanChangeDetector_TransientSpike(t *testing.T) {
	// Test that a transient spike (goes up then back down) IS detected
	// This is the key case: load goes 2->10->2, deltas are [+8, -8], mean delta=0
	// But the volatility (mean of |delta|) should catch it
	detector := NewMeanChangeDetector()

	key := generateContextKey("system.load.1", "", []string{})
	history := &mh.MetricHistory{
		Key: mh.SeriesKey{
			ContextKey: key,
			Name:       "system.load.1",
			Tags:       []string{},
		},
		Type:   metrics.APIGaugeType,
		Recent: mh.NewRingBuffer[mh.DataPoint](25),
	}

	// Baseline: stable load around 2.0 (creates deltas of ~0)
	for i := 0; i < 11; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   2.0,
				Min:   2.0,
				Max:   2.0,
			},
		})
	}

	// Recent window: spike to 10.0, then back to 2.0
	// This creates deltas like: [+8, 0, 0, -8, 0] with mean ~0 but high |delta| mean
	spikeValues := []float64{10.0, 10.0, 10.0, 2.0, 2.0}
	for i, val := range spikeValues {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64((11 + i) * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   val,
				Min:   val,
				Max:   val,
			},
		})
	}

	// Run detector
	anomalies := detector.Analyze(history.Key, history)

	// Verify anomaly detected - volatility spike should catch this
	assert.NotNil(t, anomalies, "Transient spike should be detected via volatility")
	assert.Equal(t, 1, len(anomalies))
	assert.Equal(t, "spike", anomalies[0].Type)
	assert.Contains(t, anomalies[0].Message, "Volatility spike")
}

func TestMeanAndStd(t *testing.T) {
	// Test mean and std calculation
	values := []float64{10.0, 20.0, 30.0, 40.0, 50.0}
	mean, std := meanAndStd(values)

	assert.Equal(t, 30.0, mean)
	assert.InDelta(t, 14.142, std, 0.01) // sqrt(200) ≈ 14.142

	// Test with empty slice
	mean, std = meanAndStd([]float64{})
	assert.Equal(t, 0.0, mean)
	assert.Equal(t, 0.0, std)

	// Test with single value
	mean, std = meanAndStd([]float64{42.0})
	assert.Equal(t, 42.0, mean)
	assert.Equal(t, 0.0, std)
}
