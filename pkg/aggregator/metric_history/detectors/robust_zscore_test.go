// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package detectors

import (
	"testing"

	mh "github.com/DataDog/datadog-agent/pkg/aggregator/metric_history"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
)

func TestRobustZScoreDetector_Name(t *testing.T) {
	detector := NewRobustZScoreDetector()
	assert.Equal(t, "robust_zscore", detector.Name())
}

func TestRobustZScoreDetector_DetectsSpike(t *testing.T) {
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 5 // Lower for testing

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

	// Add stable baseline around 10.0 (need enough for MinDataPoints after delta)
	for i := 0; i < 15; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   10.0,
				Min:   10.0,
				Max:   10.0,
			},
		})
	}

	// Add a spike to 100.0
	history.Recent.Push(mh.DataPoint{
		Timestamp: int64(15 * 15),
		Stats: mh.SummaryStats{
			Count: 1,
			Sum:   100.0,
			Min:   100.0,
			Max:   100.0,
		},
	})

	anomalies := detector.Analyze(history.Key, history)

	assert.NotNil(t, anomalies, "Should detect spike")
	assert.GreaterOrEqual(t, len(anomalies), 1)
	assert.Equal(t, "spike", anomalies[0].Type)
	assert.Contains(t, anomalies[0].Message, "increase")
}

func TestRobustZScoreDetector_DetectsDrop(t *testing.T) {
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 5

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

	// Add stable baseline around 100.0
	for i := 0; i < 10; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   100.0,
				Min:   100.0,
				Max:   100.0,
			},
		})
	}

	// Add a drop to 10.0
	history.Recent.Push(mh.DataPoint{
		Timestamp: int64(10 * 15),
		Stats: mh.SummaryStats{
			Count: 1,
			Sum:   10.0,
			Min:   10.0,
			Max:   10.0,
		},
	})

	anomalies := detector.Analyze(history.Key, history)

	assert.NotNil(t, anomalies, "Should detect drop")
	assert.GreaterOrEqual(t, len(anomalies), 1)
	assert.Equal(t, "drop", anomalies[0].Type)
	assert.Contains(t, anomalies[0].Message, "decrease")
}

func TestRobustZScoreDetector_NoFalsePositivesOnStableData(t *testing.T) {
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 5

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

	// Add completely stable data
	for i := 0; i < 15; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   50.0,
				Min:   50.0,
				Max:   50.0,
			},
		})
	}

	anomalies := detector.Analyze(history.Key, history)
	assert.Nil(t, anomalies, "Should not flag stable data")
}

func TestRobustZScoreDetector_NoFalsePositivesOnMonotonicCounter(t *testing.T) {
	// Key test: monotonic counters like system.uptime should NOT trigger
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 5

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

	// Simulate uptime: starts at 5270000, increases by 15 each interval
	baseValue := 5270000.0
	increment := 15.0
	for i := 0; i < 20; i++ {
		value := baseValue + float64(i)*increment
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   value,
				Min:   value,
				Max:   value,
			},
		})
	}

	anomalies := detector.Analyze(history.Key, history)
	assert.Nil(t, anomalies, "Monotonic counter with constant rate should NOT be flagged")
}

func TestRobustZScoreDetector_HandlesNormalVariation(t *testing.T) {
	// Data with normal variation should not trigger
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 5

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

	// Normal load variation between 1.5 and 2.5
	values := []float64{2.0, 2.1, 1.9, 2.2, 1.8, 2.0, 2.3, 1.7, 2.1, 1.9, 2.0, 2.2, 1.8, 2.1, 2.0}
	for i, val := range values {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   val,
				Min:   val,
				Max:   val,
			},
		})
	}

	anomalies := detector.Analyze(history.Key, history)
	assert.Nil(t, anomalies, "Normal variation should not trigger anomaly")
}

func TestRobustZScoreDetector_DetectsTransientSpike(t *testing.T) {
	// A spike that goes up and comes back down should be detected
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 5

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

	// Stable baseline
	for i := 0; i < 10; i++ {
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

	// Spike up to 15.0
	history.Recent.Push(mh.DataPoint{
		Timestamp: int64(10 * 15),
		Stats: mh.SummaryStats{
			Count: 1,
			Sum:   15.0,
			Min:   15.0,
			Max:   15.0,
		},
	})

	// Back down to 2.0
	history.Recent.Push(mh.DataPoint{
		Timestamp: int64(11 * 15),
		Stats: mh.SummaryStats{
			Count: 1,
			Sum:   2.0,
			Min:   2.0,
			Max:   2.0,
		},
	})

	anomalies := detector.Analyze(history.Key, history)

	// Should detect at least the spike (and possibly the drop back)
	assert.NotNil(t, anomalies, "Transient spike should be detected")
	assert.GreaterOrEqual(t, len(anomalies), 1)
}

func TestRobustZScoreDetector_InsufficientData(t *testing.T) {
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 10

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

	// Add only 5 points (less than MinDataPoints)
	for i := 0; i < 5; i++ {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   10.0,
				Min:   10.0,
				Max:   10.0,
			},
		})
	}

	anomalies := detector.Analyze(history.Key, history)
	assert.Nil(t, anomalies, "Should not analyze with insufficient data")
}

func TestRobustZScoreDetector_ResistantToOutliersInBaseline(t *testing.T) {
	// Key advantage of robust Z-score: outliers in baseline don't break detection
	detector := NewRobustZScoreDetector()
	detector.MinDataPoints = 5

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

	// Baseline with a few outliers mixed in
	// Normal: 10, outliers: 50
	values := []float64{10, 10, 50, 10, 10, 10, 50, 10, 10, 10, 10, 10, 10, 10}
	for i, val := range values {
		history.Recent.Push(mh.DataPoint{
			Timestamp: int64(i * 15),
			Stats: mh.SummaryStats{
				Count: 1,
				Sum:   val,
				Min:   val,
				Max:   val,
			},
		})
	}

	// Add a new massive spike - should still be detected despite outliers in baseline
	history.Recent.Push(mh.DataPoint{
		Timestamp: int64(14 * 15),
		Stats: mh.SummaryStats{
			Count: 1,
			Sum:   200.0,
			Min:   200.0,
			Max:   200.0,
		},
	})

	anomalies := detector.Analyze(history.Key, history)
	assert.NotNil(t, anomalies, "Should detect spike even with outliers in baseline")
}

func TestComputeMedian(t *testing.T) {
	// Odd number of elements
	assert.Equal(t, 3.0, computeMedian([]float64{1, 3, 5}))
	assert.Equal(t, 3.0, computeMedian([]float64{5, 1, 3}))

	// Even number of elements
	assert.Equal(t, 2.5, computeMedian([]float64{1, 2, 3, 4}))
	assert.Equal(t, 2.5, computeMedian([]float64{4, 1, 3, 2}))

	// Single element
	assert.Equal(t, 42.0, computeMedian([]float64{42}))

	// Empty slice
	assert.Equal(t, 0.0, computeMedian([]float64{}))
}

func TestComputeMAD(t *testing.T) {
	// Simple case: [1, 2, 3, 4, 5], median=3
	// Absolute deviations: [2, 1, 0, 1, 2]
	// MAD = median of [0, 1, 1, 2, 2] = 1
	values := []float64{1, 2, 3, 4, 5}
	median := computeMedian(values)
	mad := computeMAD(values, median)
	assert.Equal(t, 3.0, median)
	assert.Equal(t, 1.0, mad)

	// Constant data: MAD should be 0
	constValues := []float64{5, 5, 5, 5, 5}
	constMedian := computeMedian(constValues)
	constMAD := computeMAD(constValues, constMedian)
	assert.Equal(t, 5.0, constMedian)
	assert.Equal(t, 0.0, constMAD)
}
