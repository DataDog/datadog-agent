// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockDetector is a simple test detector that always returns a fixed anomaly for testing.
type mockDetector struct {
	name           string
	returnAnomaly  bool
	callCount      int
	lastSeriesKey  SeriesKey
	lastHistory    *MetricHistory
}

func (m *mockDetector) Name() string {
	return m.name
}

func (m *mockDetector) Analyze(key SeriesKey, history *MetricHistory) []Anomaly {
	m.callCount++
	m.lastSeriesKey = key
	m.lastHistory = history

	if m.returnAnomaly {
		return []Anomaly{{
			SeriesKey:    key,
			DetectorName: m.name,
			Timestamp:    1234567890,
			Type:         "test",
			Severity:     0.5,
			Message:      "Test anomaly",
		}}
	}
	return nil
}

func TestDetectorRegistry_Register(t *testing.T) {
	registry := NewDetectorRegistry()
	assert.NotNil(t, registry)
	assert.Equal(t, 0, len(registry.detectors))

	// Register a detector
	detector1 := &mockDetector{name: "detector1", returnAnomaly: false}
	registry.Register(detector1)
	assert.Equal(t, 1, len(registry.detectors))

	// Register another detector
	detector2 := &mockDetector{name: "detector2", returnAnomaly: false}
	registry.Register(detector2)
	assert.Equal(t, 2, len(registry.detectors))
}

func TestDetectorRegistry_RunAll_NoAnomalies(t *testing.T) {
	cache := NewMetricHistoryCache()
	registry := NewDetectorRegistry()

	// Add some series to the cache
	key1 := generateContextKey("test.metric.1", "", []string{"env:test"})
	cache.series[key1] = &MetricHistory{
		Key: SeriesKey{
			ContextKey: key1,
			Name:       "test.metric.1",
			Tags:       []string{"env:test"},
		},
		Recent: NewRingBuffer[DataPoint](20),
	}
	key2 := generateContextKey("test.metric.2", "", []string{"env:prod"})
	cache.series[key2] = &MetricHistory{
		Key: SeriesKey{
			ContextKey: key2,
			Name:       "test.metric.2",
			Tags:       []string{"env:prod"},
		},
		Recent: NewRingBuffer[DataPoint](20),
	}

	// Register detectors that don't find anomalies
	detector1 := &mockDetector{name: "detector1", returnAnomaly: false}
	detector2 := &mockDetector{name: "detector2", returnAnomaly: false}
	registry.Register(detector1)
	registry.Register(detector2)

	// Run all detectors
	anomalies := registry.RunAll(cache)

	// Verify no anomalies detected
	assert.Equal(t, 0, len(anomalies))

	// Verify each detector was called for each series
	assert.Equal(t, 2, detector1.callCount, "detector1 should be called once per series")
	assert.Equal(t, 2, detector2.callCount, "detector2 should be called once per series")
}

func TestDetectorRegistry_RunAll_WithAnomalies(t *testing.T) {
	cache := NewMetricHistoryCache()
	registry := NewDetectorRegistry()

	// Add some series to the cache
	key1 := generateContextKey("test.metric.1", "", []string{"env:test"})
	cache.series[key1] = &MetricHistory{
		Key: SeriesKey{
			ContextKey: key1,
			Name:       "test.metric.1",
			Tags:       []string{"env:test"},
		},
		Recent: NewRingBuffer[DataPoint](20),
	}
	key2 := generateContextKey("test.metric.2", "", []string{"env:prod"})
	cache.series[key2] = &MetricHistory{
		Key: SeriesKey{
			ContextKey: key2,
			Name:       "test.metric.2",
			Tags:       []string{"env:prod"},
		},
		Recent: NewRingBuffer[DataPoint](20),
	}

	// Register detectors where one returns anomalies
	detector1 := &mockDetector{name: "detector1", returnAnomaly: true}
	detector2 := &mockDetector{name: "detector2", returnAnomaly: false}
	registry.Register(detector1)
	registry.Register(detector2)

	// Run all detectors
	anomalies := registry.RunAll(cache)

	// Verify anomalies detected
	// detector1 returns 1 anomaly per series, so 2 total
	assert.Equal(t, 2, len(anomalies))

	// Verify each detector was called for each series
	assert.Equal(t, 2, detector1.callCount, "detector1 should be called once per series")
	assert.Equal(t, 2, detector2.callCount, "detector2 should be called once per series")

	// Verify anomaly properties
	for _, anomaly := range anomalies {
		assert.Equal(t, "detector1", anomaly.DetectorName)
		assert.Equal(t, "test", anomaly.Type)
		assert.Equal(t, 0.5, anomaly.Severity)
		assert.Equal(t, "Test anomaly", anomaly.Message)
	}
}

func TestDetectorRegistry_RunAll_EmptyCache(t *testing.T) {
	cache := NewMetricHistoryCache()
	registry := NewDetectorRegistry()

	// Register detectors
	detector1 := &mockDetector{name: "detector1", returnAnomaly: true}
	registry.Register(detector1)

	// Run all detectors on empty cache
	anomalies := registry.RunAll(cache)

	// Verify no anomalies detected (no series to analyze)
	assert.Equal(t, 0, len(anomalies))
	assert.Equal(t, 0, detector1.callCount, "detector should not be called for empty cache")
}

// multiAnomalyDetector is a detector that returns multiple anomalies for testing
type multiAnomalyDetector struct {
	name string
}

func (m *multiAnomalyDetector) Name() string {
	return m.name
}

func (m *multiAnomalyDetector) Analyze(key SeriesKey, history *MetricHistory) []Anomaly {
	return []Anomaly{
		{SeriesKey: key, DetectorName: m.name, Type: "anomaly1"},
		{SeriesKey: key, DetectorName: m.name, Type: "anomaly2"},
	}
}

func TestDetectorRegistry_RunAll_MultipleAnomaliesPerDetector(t *testing.T) {
	cache := NewMetricHistoryCache()
	registry := NewDetectorRegistry()

	// Add one series
	key := generateContextKey("test.metric", "", []string{"env:test"})
	cache.series[key] = &MetricHistory{
		Key: SeriesKey{
			ContextKey: key,
			Name:       "test.metric",
			Tags:       []string{"env:test"},
		},
		Recent: NewRingBuffer[DataPoint](20),
	}

	// Create a detector that returns multiple anomalies
	multiDetector := &multiAnomalyDetector{name: "multi"}
	registry.Register(multiDetector)

	// Run all detectors
	anomalies := registry.RunAll(cache)

	// Verify multiple anomalies from single detector
	assert.Equal(t, 2, len(anomalies))
	assert.Equal(t, "anomaly1", anomalies[0].Type)
	assert.Equal(t, "anomaly2", anomalies[1].Type)
}
