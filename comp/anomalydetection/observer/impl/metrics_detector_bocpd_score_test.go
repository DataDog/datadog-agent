// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBOCPDScorePriorWindow(t *testing.T) {
	for _, cadence := range []int64{1, 15} {
		storage := newDetectorTestStorage()
		for i := int64(1); i <= 60; i++ {
			storage.Add("ns", "metric", 100+float64(i%3-1), i*cadence, nil)
		}
		// Neither a candidate bucket nor future data may affect its reference.
		storage.Add("ns", "metric", 9999, 61*cadence, nil)
		storage.Add("ns", "metric", 999999, 62*cadence, nil)
		for _, reader := range []observer.StorageReader{storage, newInstrumentedStorage(storage)} {
			w := &bocpdScoreWorkspace{}
			require.Nil(t, w.score(reader, 0, observer.AggregateAverage, 60*cadence, 130, 1))
			score := w.score(reader, 0, observer.AggregateAverage, 61*cadence, 130, 1)
			require.NotNil(t, score)
			assert.InDelta(t, 30/1.4826, *score, 1e-12)
			// A different candidate value must use the same unpolluted history.
			score = w.score(reader, 0, observer.AggregateAverage, 61*cadence, 70, 1)
			require.NotNil(t, score)
			assert.InDelta(t, 30/1.4826, *score, 1e-12)
		}
	}
}

func TestBOCPDScoreNewestWindowAndAggregations(t *testing.T) {
	storage := newDetectorTestStorage()
	for i := int64(1); i <= 120; i++ {
		value := 1.0
		if i > 60 {
			value = 100
		}
		storage.Add("ns", "metric", value, i, nil)
		storage.Add("ns", "metric", value, i, nil)
	}
	for _, reader := range []observer.StorageReader{storage, newInstrumentedStorage(storage)} {
		w := &bocpdScoreWorkspace{}
		score := w.score(reader, 0, observer.AggregateAverage, 121, 130, 1)
		require.NotNil(t, score)
		assert.Equal(t, 30.0, *score)
		score = w.score(reader, 0, observer.AggregateCount, 121, 7, 1)
		require.NotNil(t, score)
		assert.Equal(t, 5.0, *score)
		// Configured observation-noise floor is in standard-deviation units.
		score = w.score(reader, 0, observer.AggregateAverage, 121, 130, 5)
		require.NotNil(t, score)
		assert.Equal(t, 6.0, *score)
	}
}

func TestBOCPDScoreNonFinite(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		storage := newDetectorTestStorage()
		for i := int64(1); i <= 60; i++ {
			storage.Add("ns", "metric", 100, i, nil)
		}
		w := &bocpdScoreWorkspace{}
		assert.Nil(t, w.score(storage, 0, observer.AggregateAverage, 61, bad, 1))
		assert.Nil(t, w.score(storage, 0, observer.AggregateAverage, 61, 100, bad))
		// Native ingestion rejects non-finite observations. Exercise the guard
		// with a reader that exposes one rather than relying on rejected input.
		reader := nonFiniteBOCPDScoreReader{StorageReader: storage, value: bad}
		assert.Nil(t, w.score(reader, 0, observer.AggregateAverage, 61, 100, 1))
	}
	storage := newDetectorTestStorage()
	for i := int64(1); i <= 60; i++ {
		storage.Add("ns", "metric", math.Copysign(math.MaxFloat64, float64(i%2)*2-1), i, nil)
	}
	assert.Nil(t, (&bocpdScoreWorkspace{}).score(storage, 0, observer.AggregateAverage, 61, 100, 1), "overflow is unavailable, not a zero score")
}

type nonFiniteBOCPDScoreReader struct {
	observer.StorageReader
	value float64
}

func (r nonFiniteBOCPDScoreReader) ForEachPoint(ref observer.SeriesRef, start, end int64, agg observer.Aggregate, fn func(*observer.Series, observer.Point)) bool {
	return r.StorageReader.ForEachPoint(ref, start, end, agg, func(series *observer.Series, p observer.Point) {
		if p.Timestamp == 60 {
			p.Value = r.value
		}
		fn(series, p)
	})
}

func TestBOCPDScoreDetectorIntegration(t *testing.T) {
	config := DefaultBOCPDConfig()
	config.Aggregations = []observer.Aggregate{observer.AggregateAverage}
	d := NewBOCPDDetector(config)
	storage := newDetectorTestStorage()
	for i := int64(1); i <= 60; i++ {
		storage.Add("ns", "metric", 100, i, nil)
	}
	require.Empty(t, d.Detect(storage, 60).Anomalies)
	require.Nil(t, d.scoreWorkspace, "quiet detectors must not allocate score scratch")
	storage.Add("ns", "metric", 140, 61, nil)
	got := d.Detect(storage, 61).Anomalies
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Score)
	assert.Equal(t, 40.0, *got[0].Score)
	assert.Equal(t, int64(61), got[0].Timestamp)
	assert.Equal(t, 2, anomalyLevel(got[0], DefaultAnomalyScorerConfig().AnomalyScorerConfig), "default remains medium despite a large score")
	assert.Equal(t, 1.0, contributorWeight(got[0], DefaultAnomalyScorerConfig().AnomalyScorerConfig))

	d.Reset()
	storage = newDetectorTestStorage()
	config.WarmupPoints = 12
	d = NewBOCPDDetector(config)
	for i := int64(1); i <= 12; i++ {
		storage.Add("ns", "metric", 100, i, nil)
	}
	d.Detect(storage, 12)
	storage.Add("ns", "metric", 140, 13, nil)
	got = d.Detect(storage, 13).Anomalies
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Score, "a real early detection still has no MAD reference")
}

func TestBOCPDScoreScorerFallbackAndBoundaries(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig().AnomalyScorerConfig
	// Experimental boundaries are opt-in and are not added to the defaults.
	_, exists := cfg.DetectorThresholds["bocpd"]
	require.False(t, exists)
	cfg.DetectorThresholds["bocpd"] = [4]float64{2, 3, 6, 10}
	for _, tc := range []struct {
		score float64
		level int
	}{{0, 0}, {1.99, 0}, {2, 1}, {3, 2}, {5.99, 2}, {6, 3}, {9.99, 3}, {10, 4}, {100, 4}} {
		score := tc.score
		a := observer.Anomaly{DetectorName: "bocpd", Score: &score}
		assert.Equal(t, tc.level, anomalyLevel(a, cfg))
	}
	for _, score := range []*float64{nil, scorePtr(math.NaN()), scorePtr(math.Inf(1))} {
		a := observer.Anomaly{DetectorName: "bocpd", Score: score}
		assert.Equal(t, 2, anomalyLevel(a, cfg))
		assert.Equal(t, 1.0, contributorWeight(a, cfg))
	}
	assert.Equal(t, 0, anomalyLevel(observer.Anomaly{DetectorName: "holt_residual"}, cfg), "other detector fallback is unchanged")
}

var bocpdScoreBenchmarkSink *float64

func BenchmarkBOCPDScore(b *testing.B) {
	storage := newDetectorTestStorage()
	for i := int64(1); i <= 120; i++ {
		storage.Add("ns", "metric", 100+float64(i%3-1), i, nil)
	}
	w := &bocpdScoreWorkspace{}
	w.score(storage, 0, observer.AggregateAverage, 121, 130, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bocpdScoreBenchmarkSink = w.score(storage, 0, observer.AggregateAverage, 121, 130, 1)
	}
}
