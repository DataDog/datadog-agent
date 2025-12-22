// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

func TestSpikeDetector_Name(t *testing.T) {
	d := &SpikeDetector{}
	assert.Equal(t, "spike_detector", d.Name())
}

func TestSpikeDetector_NotEnoughPoints(t *testing.T) {
	d := &SpikeDetector{}

	// Empty series
	result := d.Analyze(observer.Series{})
	assert.Empty(t, result.Anomalies)

	// Only one point
	result = d.Analyze(observer.Series{
		Points: []observer.Point{{Value: 10}},
	})
	assert.Empty(t, result.Anomalies)
}

func TestSpikeDetector_NoSpike(t *testing.T) {
	d := &SpikeDetector{}

	// Stable values - no spike
	series := observer.Series{
		Namespace: "test",
		Name:      "my.metric",
		Tags:      []string{"env:prod"},
		Points: []observer.Point{
			{Timestamp: 1, Value: 10},
			{Timestamp: 2, Value: 12},
			{Timestamp: 3, Value: 11},
		},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestSpikeDetector_Spike(t *testing.T) {
	d := &SpikeDetector{}

	// Last value is > 2x average of prior values
	series := observer.Series{
		Namespace: "test",
		Name:      "my.metric",
		Tags:      []string{"env:prod"},
		Points: []observer.Point{
			{Timestamp: 1, Value: 10},
			{Timestamp: 2, Value: 10},
			{Timestamp: 3, Value: 10},
			{Timestamp: 4, Value: 50}, // 50 > 2*10
		},
	}

	result := d.Analyze(series)
	assert.Len(t, result.Anomalies, 1)
	assert.Equal(t, "Spike detected", result.Anomalies[0].Title)
	assert.Contains(t, result.Anomalies[0].Description, "test/my.metric")
	assert.Contains(t, result.Anomalies[0].Description, "50.00")
	assert.Equal(t, []string{"env:prod"}, result.Anomalies[0].Tags)
}

func TestSpikeDetector_ExactlyDoubleIsNotSpike(t *testing.T) {
	d := &SpikeDetector{}

	// Last value is exactly 2x - not a spike (we need > 2x)
	series := observer.Series{
		Points: []observer.Point{
			{Value: 10},
			{Value: 20}, // exactly 2x
		},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestSpikeDetector_ZeroAverage(t *testing.T) {
	d := &SpikeDetector{}

	// Zero average - should not flag
	series := observer.Series{
		Points: []observer.Point{
			{Value: 0},
			{Value: 0},
			{Value: 10},
		},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}
