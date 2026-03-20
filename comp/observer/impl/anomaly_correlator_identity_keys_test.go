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

func TestLeadLagCorrelator_PreservesFullSeriesIDs(t *testing.T) {
	c := NewLeadLagCorrelator(LeadLagConfig{
		MaxLagSeconds:       30,
		MinObservations:     1,
		ConfidenceThreshold: 0.5,
		MaxSourceTimestamps: 10,
		WindowSeconds:       120,
	})

	seriesA := observer.SeriesRef(0)
	seriesB := observer.SeriesRef(1)

	c.ProcessAnomaly(observer.Anomaly{SourceView: observer.QueryHandle{Ref: seriesA, Aggregate: observer.AggregateAverage}, Timestamp: 100})
	c.ProcessAnomaly(observer.Anomaly{SourceView: observer.QueryHandle{Ref: seriesB, Aggregate: observer.AggregateAverage}, Timestamp: 108})

	edges := c.GetEdges()
	require.NotEmpty(t, edges)
	assert.Contains(t, []string{"0", "1"}, edges[0].Leader)
	assert.Contains(t, []string{"0", "1"}, edges[0].Follower)
}

func TestSurpriseCorrelator_PreservesFullSeriesIDs(t *testing.T) {
	c := NewSurpriseCorrelator(SurpriseConfig{
		WindowSizeSeconds:     10,
		MinLift:               1.0,
		MaxLift:               0.5,
		MinSupport:            1,
		MinSourceCount:        1,
		MaxPairsTracked:       100,
		EvictionWindowSeconds: 300,
	})

	seriesA := observer.SeriesRef(0)
	seriesB := observer.SeriesRef(1)

	c.ProcessAnomaly(observer.Anomaly{SourceView: observer.QueryHandle{Ref: seriesA, Aggregate: observer.AggregateAverage}, Timestamp: 100})
	c.ProcessAnomaly(observer.Anomaly{SourceView: observer.QueryHandle{Ref: seriesB, Aggregate: observer.AggregateAverage}, Timestamp: 101})
	c.Advance(200) // finalize window

	edges := c.GetEdges()
	require.NotEmpty(t, edges)
	assert.Contains(t, []string{"0", "1"}, edges[0].Source1)
	assert.Contains(t, []string{"0", "1"}, edges[0].Source2)
}
