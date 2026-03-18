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

func TestDetectorPassthroughCorrelator_OnePerAnomaly(t *testing.T) {
	c := NewDetectorPassthroughCorrelator()

	c.ProcessAnomaly(observer.Anomaly{DetectorName: "cusum", Source: "redis.cpu.sys", SourceSeriesID: "s1", Timestamp: 100})
	c.ProcessAnomaly(observer.Anomaly{DetectorName: "bocpd", Source: "redis.cpu.sys", SourceSeriesID: "s1", Timestamp: 105})
	c.ProcessAnomaly(observer.Anomaly{DetectorName: "cusum", Source: "redis.info.latency_ms", SourceSeriesID: "s2", Timestamp: 110})

	corrs := c.ActiveCorrelations()
	// 3 anomalies = 3 correlations (one per anomaly)
	require.Len(t, corrs, 3)

	// Sorted by detector name (bocpd before cusum), then by timestamp within detector
	assert.Equal(t, "passthrough_bocpd_0", corrs[0].Pattern)
	assert.Equal(t, int64(105), corrs[0].FirstSeen)
	assert.Len(t, corrs[0].Anomalies, 1)

	assert.Equal(t, "passthrough_cusum_0", corrs[1].Pattern)
	assert.Equal(t, int64(100), corrs[1].FirstSeen)

	assert.Equal(t, "passthrough_cusum_1", corrs[2].Pattern)
	assert.Equal(t, int64(110), corrs[2].FirstSeen)
}

func TestDetectorPassthroughCorrelator_TimestampOrdering(t *testing.T) {
	c := NewDetectorPassthroughCorrelator()

	// Process out of order
	c.ProcessAnomaly(observer.Anomaly{DetectorName: "pelt", Timestamp: 300})
	c.ProcessAnomaly(observer.Anomaly{DetectorName: "pelt", Timestamp: 150})
	c.ProcessAnomaly(observer.Anomaly{DetectorName: "pelt", Timestamp: 200})

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 3)
	// Should be sorted by timestamp within the detector
	assert.Equal(t, int64(150), corrs[0].FirstSeen)
	assert.Equal(t, int64(200), corrs[1].FirstSeen)
	assert.Equal(t, int64(300), corrs[2].FirstSeen)
}

func TestDetectorPassthroughCorrelator_SeriesIDAndSource(t *testing.T) {
	c := NewDetectorPassthroughCorrelator()

	c.ProcessAnomaly(observer.Anomaly{DetectorName: "cusum", Source: "redis.cpu.sys", SourceSeriesID: "s1", Timestamp: 100})

	corrs := c.ActiveCorrelations()
	require.Len(t, corrs, 1)
	assert.Equal(t, []observer.SeriesID{"s1"}, corrs[0].MemberSeriesIDs)
	assert.Equal(t, []observer.MetricName{"redis.cpu.sys"}, corrs[0].MetricNames)
}

func TestDetectorPassthroughCorrelator_Reset(t *testing.T) {
	c := NewDetectorPassthroughCorrelator()

	c.ProcessAnomaly(observer.Anomaly{DetectorName: "cusum", Timestamp: 100})
	require.Len(t, c.ActiveCorrelations(), 1)

	c.Reset()
	assert.Empty(t, c.ActiveCorrelations())
}

func TestDetectorPassthroughCorrelator_Empty(t *testing.T) {
	c := NewDetectorPassthroughCorrelator()
	assert.Empty(t, c.ActiveCorrelations())
}

func TestDetectorPassthroughCorrelator_ImplementsCorrelator(t *testing.T) {
	var _ observer.Correlator = NewDetectorPassthroughCorrelator()
}
