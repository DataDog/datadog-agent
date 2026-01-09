// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeClusterCorrelator_BasicClustering(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		SlackSeconds:   5,
		MinClusterSize: 2,
		WindowSeconds:  60,
	})

	// Two anomalies with overlapping time ranges should cluster together
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		TimeRange: observer.TimeRange{Start: 100, End: 110},
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		TimeRange: observer.TimeRange{Start: 105, End: 115},
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	assert.Contains(t, correlations[0].Signals, "metric.a")
	assert.Contains(t, correlations[0].Signals, "metric.b")
}

func TestTimeClusterCorrelator_SlackWindow(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		SlackSeconds:   5,
		MinClusterSize: 2,
		WindowSeconds:  60,
	})

	// Anomalies within slack window should cluster
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		TimeRange: observer.TimeRange{Start: 100, End: 105},
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		TimeRange: observer.TimeRange{Start: 108, End: 115}, // starts 3s after A ends, within 5s slack
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
}

func TestTimeClusterCorrelator_NoOverlap(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		SlackSeconds:   5,
		MinClusterSize: 2,
		WindowSeconds:  60,
	})

	// Anomalies outside slack window should NOT cluster
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		TimeRange: observer.TimeRange{Start: 100, End: 105},
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		TimeRange: observer.TimeRange{Start: 120, End: 130}, // starts 15s after A ends, outside 5s slack
	})

	correlations := c.ActiveCorrelations()
	// Neither cluster meets MinClusterSize of 2
	assert.Len(t, correlations, 0)
}

func TestTimeClusterCorrelator_MergeClusters(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		SlackSeconds:   5,
		MinClusterSize: 2,
		WindowSeconds:  60,
	})

	// Create two separate clusters
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		TimeRange: observer.TimeRange{Start: 100, End: 105},
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		TimeRange: observer.TimeRange{Start: 120, End: 125},
	})

	// Verify two separate clusters (both below threshold)
	assert.Len(t, c.clusters, 2)

	// Add anomaly that bridges both clusters
	c.Process(observer.AnomalyOutput{
		Source:    "metric.c",
		Title:     "Anomaly C",
		TimeRange: observer.TimeRange{Start: 103, End: 122}, // overlaps both
	})

	// Should now be merged into one cluster
	assert.Len(t, c.clusters, 1)
	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 3)
}

func TestTimeClusterCorrelator_DedupBySource(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		SlackSeconds:   5,
		MinClusterSize: 1, // Lower threshold to see single-anomaly clusters
		WindowSeconds:  60,
	})

	// Same source, later anomaly should replace earlier
	c.Process(observer.AnomalyOutput{
		Source:      "metric.a",
		Title:       "Anomaly A v1",
		Description: "first",
		TimeRange:   observer.TimeRange{Start: 100, End: 110},
	})
	c.Process(observer.AnomalyOutput{
		Source:      "metric.a",
		Title:       "Anomaly A v2",
		Description: "second",
		TimeRange:   observer.TimeRange{Start: 105, End: 120},
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 1)
	assert.Equal(t, "second", correlations[0].Anomalies[0].Description)
}

func TestTimeClusterCorrelator_Eviction(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		SlackSeconds:   5,
		MinClusterSize: 1,
		WindowSeconds:  30,
	})

	// Add old anomaly
	c.Process(observer.AnomalyOutput{
		Source:    "metric.old",
		Title:     "Old Anomaly",
		TimeRange: observer.TimeRange{Start: 100, End: 110},
	})

	// Add recent anomaly (advances currentDataTime)
	c.Process(observer.AnomalyOutput{
		Source:    "metric.new",
		Title:     "New Anomaly",
		TimeRange: observer.TimeRange{Start: 200, End: 210},
	})

	// Flush should evict the old cluster
	c.Flush()

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Equal(t, "metric.new", correlations[0].Signals[0])
}

func TestTimeClusterCorrelator_MinClusterSize(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		SlackSeconds:   5,
		MinClusterSize: 3, // Require 3 anomalies
		WindowSeconds:  60,
	})

	// Add 2 overlapping anomalies
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		TimeRange: observer.TimeRange{Start: 100, End: 110},
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		TimeRange: observer.TimeRange{Start: 105, End: 115},
	})

	// Should not report - only 2 anomalies, need 3
	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 0)

	// Add third
	c.Process(observer.AnomalyOutput{
		Source:    "metric.c",
		TimeRange: observer.TimeRange{Start: 108, End: 118},
	})

	// Now should report
	correlations = c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 3)
}
