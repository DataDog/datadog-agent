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
	result := d.Analyze(&observer.SeriesStats{})
	assert.Empty(t, result.Anomalies)

	// Only one point
	result = d.Analyze(&observer.SeriesStats{
		Points: []observer.StatPoint{{Sum: 10, Count: 1}},
	})
	assert.Empty(t, result.Anomalies)
}

func TestSpikeDetector_NoSpike(t *testing.T) {
	d := &SpikeDetector{}

	// Stable values - no spike
	series := &observer.SeriesStats{
		Namespace: "test",
		Name:      "my.metric",
		Tags:      []string{"env:prod"},
		Points: []observer.StatPoint{
			{Timestamp: 1, Sum: 10, Count: 1},
			{Timestamp: 2, Sum: 12, Count: 1},
			{Timestamp: 3, Sum: 11, Count: 1},
		},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestSpikeDetector_Spike(t *testing.T) {
	d := &SpikeDetector{}

	// Last value is > 2x average of prior values
	series := &observer.SeriesStats{
		Namespace: "test",
		Name:      "my.metric",
		Tags:      []string{"env:prod"},
		Points: []observer.StatPoint{
			{Timestamp: 1, Sum: 10, Count: 1}, // 10
			{Timestamp: 2, Sum: 10, Count: 1}, // 10
			{Timestamp: 3, Sum: 10, Count: 1}, // 10
			{Timestamp: 4, Sum: 50, Count: 1}, // 50 > 2*10
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
	series := &observer.SeriesStats{
		Points: []observer.StatPoint{
			{Sum: 10, Count: 1},
			{Sum: 20, Count: 1}, // exactly 2x
		},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestSpikeDetector_ZeroAverage(t *testing.T) {
	d := &SpikeDetector{}

	// Zero average - should not flag
	series := &observer.SeriesStats{
		Points: []observer.StatPoint{
			{Sum: 0, Count: 1},
			{Sum: 0, Count: 1},
			{Sum: 10, Count: 1},
		},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}
