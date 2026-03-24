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

func testDensityCorrelator(minBurst int) *DensityCorrelator {
	return NewDensityCorrelator(DensityConfig{
		ShortWindowSec:   10,
		LongWindowSec:    300,
		MinBurst:         minBurst,
		BurstMultiplier:  2.0,
		MinUniqueSources: 3,
		WindowSeconds:    120,
	})
}

func makeAnomaly(ts int64, source string) observer.Anomaly {
	return observer.Anomaly{
		Timestamp:      ts,
		Source:         observer.AnomalySource{Name: source, Aggregate: observer.AggregateAverage},
		SourceSeriesID: observer.SeriesID(source),
		DetectorName:   "test",
	}
}

func makeDiverseAnomalies(ts int64, n int) []observer.Anomaly {
	result := make([]observer.Anomaly, n)
	for i := 0; i < n; i++ {
		result[i] = makeAnomaly(ts+int64(i), "source_"+string(rune('a'+i)))
	}
	return result
}

func TestDensity_NoBurstBelowThreshold(t *testing.T) {
	c := testDensityCorrelator(5)
	for i := 0; i < 4; i++ {
		c.ProcessAnomaly(makeAnomaly(100+int64(i), "metric_"+string(rune('a'+i))))
	}
	assert.Empty(t, c.ActiveCorrelations())
}

func TestDensity_BurstTriggersAtThreshold(t *testing.T) {
	c := testDensityCorrelator(5)
	for _, a := range makeDiverseAnomalies(100, 5) {
		c.ProcessAnomaly(a)
	}
	corr := c.ActiveCorrelations()
	require.Len(t, corr, 1)
	// FirstSeen = trigger timestamp of the anomaly that crossed threshold.
	assert.Equal(t, int64(104), corr[0].FirstSeen)
}

func TestDensity_TwoSeparateBursts(t *testing.T) {
	c := testDensityCorrelator(5)

	for _, a := range makeDiverseAnomalies(100, 10) {
		c.ProcessAnomaly(a)
	}
	for _, a := range makeDiverseAnomalies(200, 10) {
		c.ProcessAnomaly(a)
	}
	corr := c.ActiveCorrelations()
	assert.Len(t, corr, 2, "two non-overlapping bursts should both be reported")
}

func TestDensity_OverlappingBurstDeduped(t *testing.T) {
	c := testDensityCorrelator(5)

	for _, a := range makeDiverseAnomalies(100, 5) {
		c.ProcessAnomaly(a)
	}
	require.Len(t, c.ActiveCorrelations(), 1)

	// More anomalies in overlapping window — should not create second burst.
	for _, a := range makeDiverseAnomalies(105, 5) {
		c.ProcessAnomaly(a)
	}
	assert.Len(t, c.ActiveCorrelations(), 1, "overlapping burst should be deduped")
}

func TestDensity_LateAnomalyWithinExtensionMergesIntoBurst(t *testing.T) {
	// Burst triggers at t=100 (windowEnd=100, windowStart=90).
	// A late anomaly arriving at t=109 falls in (windowEnd=100, windowEnd+ShortWindowSec=110]
	// — within the extension window. It should merge into the existing burst,
	// not create a second one.
	c := testDensityCorrelator(5)

	for _, a := range makeDiverseAnomalies(100, 5) {
		c.ProcessAnomaly(a)
	}
	require.Len(t, c.ActiveCorrelations(), 1)

	// t=109 is 9s past windowEnd=100, inside the ShortWindowSec=10 extension.
	c.ProcessAnomaly(makeAnomaly(109, "late_source"))
	assert.Len(t, c.ActiveCorrelations(), 1, "late anomaly within extension should merge, not create new burst")

	// t=111 is 11s past windowEnd=100, outside the extension — new burst territory.
	// Feed enough anomalies to trigger a second burst.
	for _, a := range makeDiverseAnomalies(111, 5) {
		c.ProcessAnomaly(a)
	}
	assert.Len(t, c.ActiveCorrelations(), 2, "anomalies beyond extension window should form a new burst")
}

func TestDensity_SlowSamplingAnomalyMergesViaSamplingInterval(t *testing.T) {
	// Burst triggers at t=100 from fast-sampling anomalies (ShortWindowSec=10).
	// A slow-sampling anomaly (SamplingIntervalSec=15) arrives at t=112,
	// which is 12s past windowEnd=100. The fixed extension (ShortWindowSec=10)
	// would miss it (12 > 10), but the incoming anomaly's SamplingIntervalSec=15
	// widens the extension to 15, so it merges (12 <= 15).
	c := testDensityCorrelator(5)

	for _, a := range makeDiverseAnomalies(100, 5) {
		c.ProcessAnomaly(a)
	}
	require.Len(t, c.ActiveCorrelations(), 1)

	slowAnomaly := makeAnomaly(112, "redis.info.latency_ms")
	slowAnomaly.SamplingIntervalSec = 15
	c.ProcessAnomaly(slowAnomaly)
	assert.Len(t, c.ActiveCorrelations(), 1, "slow-sampling anomaly should merge into burst via SamplingIntervalSec extension")

	// t=116 with SamplingIntervalSec=0 is 16s past windowEnd=100 — beyond both
	// ShortWindowSec=10 and the burst's maxSamplingInterval=15 → new burst territory.
	for _, a := range makeDiverseAnomalies(116, 5) {
		c.ProcessAnomaly(a)
	}
	assert.Len(t, c.ActiveCorrelations(), 2, "anomalies beyond sampling interval extension should form a new burst")
}

func TestDensity_FirstSeenIsTriggerTimestamp(t *testing.T) {
	c := testDensityCorrelator(5)

	// All 5 anomalies fall within the ShortWindowSec=10 window [90,100].
	// The burst triggers when the 5th anomaly (ts=100) crosses MinBurst=5.
	// FirstSeen = 100 (trigger), Anomalies includes the full window [90,100].
	c.ProcessAnomaly(makeAnomaly(90, "src_a"))
	c.ProcessAnomaly(makeAnomaly(95, "src_b"))
	c.ProcessAnomaly(makeAnomaly(97, "src_c"))
	c.ProcessAnomaly(makeAnomaly(98, "src_d"))
	c.ProcessAnomaly(makeAnomaly(100, "src_e")) // triggers

	corr := c.ActiveCorrelations()
	require.Len(t, corr, 1)
	assert.Equal(t, int64(100), corr[0].FirstSeen)
	// Anomalies include everything in [shortCutoff=90, triggerTS=100] — the burst evidence.
	assert.Len(t, corr[0].Anomalies, 5)
	for _, a := range corr[0].Anomalies {
		assert.GreaterOrEqual(t, a.Timestamp, int64(90), "anomaly before short window")
		assert.LessOrEqual(t, a.Timestamp, int64(100), "anomaly after trigger")
	}
}

func TestDensity_DiversityFilterRejectsLowDiversity(t *testing.T) {
	c := testDensityCorrelator(5)
	// 10 anomalies from only 2 sources — below MinUniqueSources=3.
	for i := 0; i < 10; i++ {
		src := "metric_a"
		if i%2 == 0 {
			src = "metric_b"
		}
		c.ProcessAnomaly(makeAnomaly(100+int64(i), src))
	}
	assert.Empty(t, c.ActiveCorrelations(), "low diversity should not trigger")
}

func TestDensity_EvictionRemovesOldBursts(t *testing.T) {
	c := testDensityCorrelator(5)

	// Burst at t=100.
	for _, a := range makeDiverseAnomalies(100, 10) {
		c.ProcessAnomaly(a)
	}
	require.Len(t, c.ActiveCorrelations(), 1)

	// Push currentDataTime forward by adding a later anomaly, then advance.
	// Eviction uses currentDataTime (anomaly-driven), not Advance's dataTime.
	c.ProcessAnomaly(makeAnomaly(230, "late_metric"))
	c.Advance(230)
	assert.Empty(t, c.ActiveCorrelations(), "old burst should be evicted")
}

func TestDensity_Reset(t *testing.T) {
	c := testDensityCorrelator(5)
	for _, a := range makeDiverseAnomalies(100, 10) {
		c.ProcessAnomaly(a)
	}
	require.Len(t, c.ActiveCorrelations(), 1)

	c.Reset()
	assert.Empty(t, c.ActiveCorrelations())
}
