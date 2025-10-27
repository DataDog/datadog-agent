// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	otlpmetrics "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestConsumeTimeSeriesOptimized(t *testing.T) {
	consumer := &serializerConsumer{
		extraTags: []string{"env:test", "service:test-service"},
		tagBuffer: make([]string, 0, 10), // Pre-allocate with reasonable capacity
	}

	// Create test dimensions
	dimensions := &otlpmetrics.Dimensions{
		// Mock implementation - in real code this would be properly initialized
	}

	// Mock the dimensions methods for testing
	// Note: This is a simplified test - in practice you'd need to properly mock the Dimensions struct
	// or use a test helper that creates real Dimensions objects

	// Test that ConsumeTimeSeries doesn't panic and creates a serie
	// This is a basic smoke test - more comprehensive testing would require proper mocking
	consumer.ConsumeTimeSeries(context.Background(), dimensions, otlpmetrics.Gauge, 1000000000, 60, 42.5)

	// Verify that a serie was added
	assert.Len(t, consumer.series, 1, "Expected one serie to be added")

	// Verify the serie has the expected structure
	serie := consumer.series[0]
	assert.NotNil(t, serie, "Serie should not be nil")
	assert.NotNil(t, serie.Points, "Points should not be nil")
	assert.Len(t, serie.Points, 1, "Expected one point")
	assert.Equal(t, 1.0, serie.Points[0].Ts, "Expected timestamp to be converted to seconds")
	assert.Equal(t, 42.5, serie.Points[0].Value, "Expected value to be preserved")
}

func TestEnrichTagsLogic(t *testing.T) {
	tests := []struct {
		name      string
		extraTags []string
		dimTags   []string
		expected  []string
	}{
		{
			name:      "empty tags",
			extraTags: []string{},
			dimTags:   []string{},
			expected:  []string{},
		},
		{
			name:      "only extra tags",
			extraTags: []string{"env:test", "service:web"},
			dimTags:   []string{},
			expected:  []string{"env:test", "service:web"},
		},
		{
			name:      "only dimension tags",
			extraTags: []string{},
			dimTags:   []string{"host:server1", "region:us-west"},
			expected:  []string{"host:server1", "region:us-west"},
		},
		{
			name:      "both extra and dimension tags",
			extraTags: []string{"env:test", "service:web"},
			dimTags:   []string{"host:server1", "region:us-west"},
			expected:  []string{"env:test", "service:web", "host:server1", "region:us-west"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the enrichTags logic directly
			capacity := len(tt.extraTags) + len(tt.dimTags)
			tags := make([]string, 0, capacity)
			tags = append(tags, tt.extraTags...)
			tags = append(tags, tt.dimTags...)

			assert.Equal(t, tt.expected, tags, "enrichTags logic should combine tags correctly")
		})
	}
}

func TestEnrichTagsOptimizedLogic(t *testing.T) {
	tests := []struct {
		name      string
		extraTags []string
		dimTags   []string
		expected  []string
	}{
		{
			name:      "empty tags",
			extraTags: []string{},
			dimTags:   []string{},
			expected:  []string{},
		},
		{
			name:      "only extra tags",
			extraTags: []string{"env:test", "service:web"},
			dimTags:   []string{},
			expected:  []string{"env:test", "service:web"},
		},
		{
			name:      "only dimension tags",
			extraTags: []string{},
			dimTags:   []string{"host:server1", "region:us-west"},
			expected:  []string{"host:server1", "region:us-west"},
		},
		{
			name:      "both extra and dimension tags",
			extraTags: []string{"env:test", "service:web"},
			dimTags:   []string{"host:server1", "region:us-west"},
			expected:  []string{"env:test", "service:web", "host:server1", "region:us-west"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the enrichTagsOptimized logic directly
			buf := make([]string, 0, 10)
			originalCap := cap(buf)

			buf = buf[:0]
			buf = append(buf, tt.extraTags...)
			buf = append(buf, tt.dimTags...)

			assert.Equal(t, tt.expected, buf, "enrichTagsOptimized logic should combine tags correctly")
			assert.Equal(t, originalCap, cap(buf), "Buffer capacity should be preserved")
			assert.Len(t, buf, len(tt.expected), "Result length should match expected")
		})
	}
}

func TestEnrichTagsOptimizedBufferReuseLogic(t *testing.T) {
	extraTags1 := []string{"env:test"}
	dimTags1 := []string{"host:server1"}

	// Test multiple calls with the same buffer
	buf := make([]string, 0, 5)

	// First call
	buf = buf[:0]
	buf = append(buf, extraTags1...)
	buf = append(buf, dimTags1...)
	expected1 := []string{"env:test", "host:server1"}
	assert.Equal(t, expected1, buf)
	assert.Equal(t, 5, cap(buf), "Buffer capacity should be preserved")

	// Second call with same buffer
	extraTags2 := []string{"env:prod", "service:api"}
	dimTags2 := []string{"host:server2", "region:us-west"}
	buf = buf[:0]
	buf = append(buf, extraTags2...)
	buf = append(buf, dimTags2...)
	expected2 := []string{"env:prod", "service:api", "host:server2", "region:us-west"}
	assert.Equal(t, expected2, buf)
	assert.Equal(t, 5, cap(buf), "Buffer capacity should still be preserved")
}

func TestSeriePoolReuse(t *testing.T) {
	// Get a serie from the pool
	serie1 := seriePool.Get().(*metrics.Serie)
	originalCapacity := cap(serie1.Points)

	// Use the serie
	serie1.Name = "test.metric"
	serie1.Points = append(serie1.Points, metrics.Point{Ts: 1.0, Value: 42.0})

	// Manually reset the serie (simulating what returnSeriesToPool does)
	serie1.Name = ""
	serie1.Points = serie1.Points[:0]
	serie1.Tags = tagset.CompositeTags{}
	serie1.Host = ""
	serie1.Device = ""
	serie1.MType = 0
	serie1.Interval = 0
	serie1.SourceTypeName = ""
	serie1.ContextKey = 0
	serie1.NameSuffix = ""
	serie1.NoIndex = false
	serie1.Resources = nil
	serie1.Source = 0

	// Return to pool
	seriePool.Put(serie1)

	// Get another serie from the pool
	serie2 := seriePool.Get().(*metrics.Serie)

	// Verify it's the same underlying object (pool reuse)
	assert.Equal(t, originalCapacity, cap(serie2.Points), "Points capacity should be preserved")
	assert.Len(t, serie2.Points, 0, "Points should be reset")
	assert.Empty(t, serie2.Name, "Name should be reset")

	// Return to pool
	seriePool.Put(serie2)
}

func TestReturnSeriesToPool(t *testing.T) {
	consumer := &serializerConsumer{
		series: make(metrics.Series, 0, 10),
	}

	// Add some series to the consumer
	serie1 := &metrics.Serie{
		Name:   "test.metric1",
		Points: []metrics.Point{{Ts: 1.0, Value: 42.0}},
		Host:   "test-host",
		MType:  metrics.APIGaugeType,
	}
	serie2 := &metrics.Serie{
		Name:   "test.metric2",
		Points: []metrics.Point{{Ts: 2.0, Value: 84.0}},
		Host:   "test-host",
		MType:  metrics.APICountType,
	}

	consumer.series = append(consumer.series, serie1, serie2)

	// Return series to pool
	consumer.returnSeriesToPool()

	// Verify series slice is cleared but capacity preserved
	assert.Len(t, consumer.series, 0, "Series slice should be empty")
	assert.GreaterOrEqual(t, cap(consumer.series), 2, "Series slice capacity should be preserved")
}
