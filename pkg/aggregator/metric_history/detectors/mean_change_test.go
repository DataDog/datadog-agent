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

	// Create a series with a clear changepoint
	// First half: values around 10.0
	// Second half: values around 100.0
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

	// Add first half: 10 points around 10.0
	for i := 0; i < 10; i++ {
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

	// Add second half: 10 points around 100.0
	for i := 10; i < 20; i++ {
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
	assert.Contains(t, anomaly.Message, "10.00")
	assert.Contains(t, anomaly.Message, "100.00")
}

func TestMeanChangeDetector_DetectsDecrease(t *testing.T) {
	detector := NewMeanChangeDetector()

	// Create a series with a decrease
	// First half: values around 100.0
	// Second half: values around 10.0
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

	// Add first half: 10 points around 100.0
	for i := 0; i < 10; i++ {
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

	// Add second half: 10 points around 10.0
	for i := 10; i < 20; i++ {
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
}

func TestMeanChangeDetector_NoDetectionWhenStable(t *testing.T) {
	detector := NewMeanChangeDetector()

	// Create a series with stable values
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

	// Add 20 points all around 50.0
	for i := 0; i < 20; i++ {
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
	// First half: values around 100.0
	// Second half: values around 102.0 (only 2% change, should be < 2 stddev)
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

	// Add first half: 10 points around 100.0 with slight variation
	for i := 0; i < 10; i++ {
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

	// Add second half: 10 points around 101.0 with similar variation
	for i := 10; i < 20; i++ {
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

	// Create a series with insufficient data (less than MinSegmentSize * 2)
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

	// Add only 8 points (less than default MinSegmentSize * 2 = 10)
	for i := 0; i < 8; i++ {
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
		Threshold:      5.0, // higher threshold
		MinSegmentSize: 5,
	}

	// Create a series with a moderate change that has some variance
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

	// Add first half: 10 points around 100.0 with variance (90-110)
	for i := 0; i < 10; i++ {
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

	// Add second half: 10 points around 115.0 with similar variance (105-125)
	// This is a 15-unit change with ~5-unit stddev = 3 stddev change
	// This is below our threshold of 5.0 stddev
	for i := 10; i < 20; i++ {
		value := 115.0 + float64(i%3)*5.0 - 5.0 // values: 110, 115, 120, 110, 115, 120...
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
