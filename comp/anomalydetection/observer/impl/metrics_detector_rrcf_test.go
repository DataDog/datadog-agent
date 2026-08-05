// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRCForest_DuplicateBaselinePreservesOutlierContrast(t *testing.T) {
	forest := newRCForest(20, 16, 1, 42)

	for i := 0; i < 16; i++ {
		_, score := forest.insertPoint([]float64{1})
		assert.Zero(t, score, "identical baseline points must have zero CoDisp")
	}

	_, score := forest.insertPoint([]float64{10})
	assert.Greater(t, score, 0.0, "an outlier after a constant baseline must have positive CoDisp")
}

func TestRRCFDetector_DetectsLogCountSpike(t *testing.T) {
	cfg := DefaultRRCFConfig()
	cfg.NumTrees = 20
	cfg.TreeSize = 8
	cfg.ShingleSize = 2
	detector := NewRRCFDetector(cfg)
	storage := newDetectorTestStorage()

	for ts := int64(1); ts <= 24; ts++ {
		storage.Add(LogPatternExtractorName, "log.pattern.count", 1, ts, []string{"service:test"})
	}
	for i := 0; i < 20; i++ {
		storage.Add(LogPatternExtractorName, "log.pattern.count", 1, 25, []string{"service:test"})
	}

	result := detector.Detect(storage, 25)

	require.NotEmpty(t, result.Anomalies)
	anomaly := result.Anomalies[len(result.Anomalies)-1]
	assert.Equal(t, int64(25), anomaly.Timestamp)
	assert.Equal(t, LogPatternExtractorName, anomaly.Source.Namespace)
	assert.Equal(t, observer.AggregateCount, anomaly.Source.Aggregate)
	assert.NotNil(t, anomaly.SourceRef)
	assert.NotNil(t, anomaly.Score)
	assert.Greater(t, *anomaly.Score, 0.0)
}

func TestRRCFDetector_RetriesPartialMetricResolution(t *testing.T) {
	cfg := DefaultRRCFConfig()
	cfg.NumTrees = 5
	cfg.TreeSize = 8
	cfg.ShingleSize = 1
	cfg.Metrics = []RRCFMetricDef{
		{Namespace: "test", Name: "one", Agg: observer.AggregateAverage},
		{Namespace: "test", Name: "two", Agg: observer.AggregateAverage},
	}
	detector := NewRRCFDetector(cfg)
	storage := newDetectorTestStorage()

	for ts := int64(1); ts <= 4; ts++ {
		storage.Add("test", "one", float64(ts), ts, nil)
	}
	detector.Detect(storage, 4)
	assert.Empty(t, detector.resolvedKeys)

	for ts := int64(1); ts <= 4; ts++ {
		storage.Add("test", "two", float64(ts), ts, nil)
	}
	detector.Detect(storage, 4)

	assert.Len(t, detector.resolvedKeys, 2)
	assert.Equal(t, 4, detector.GetScoreStats().SampleCount)
}

func TestRRCFDetector_IncrementalMatchesBatch(t *testing.T) {
	cfg := DefaultRRCFConfig()
	cfg.NumTrees = 10
	cfg.TreeSize = 8
	cfg.ShingleSize = 3
	cfg.ThresholdSigma = 0
	cfg.Metrics = []RRCFMetricDef{
		{Namespace: "test", Name: "one", Agg: observer.AggregateAverage},
		{Namespace: "test", Name: "two", Agg: observer.AggregateAverage},
	}

	batch := NewRRCFDetector(cfg)
	incremental := NewRRCFDetector(cfg)
	batchStorage := newDetectorTestStorage()
	incrementalStorage := newDetectorTestStorage()

	for ts := int64(1); ts <= 20; ts++ {
		batchStorage.Add("test", "one", float64(ts), ts, nil)
		batchStorage.Add("test", "two", float64(ts*2), ts, nil)
	}
	batch.Detect(batchStorage, 20)

	for ts := int64(1); ts <= 20; ts++ {
		incrementalStorage.Add("test", "one", float64(ts), ts, nil)
		incrementalStorage.Add("test", "two", float64(ts*2), ts, nil)
		incremental.Detect(incrementalStorage, ts)
	}

	batchStats := batch.GetScoreStats()
	incrementalStats := incremental.GetScoreStats()
	require.Equal(t, 18, batchStats.SampleCount)
	assert.Equal(t, batchStats.Scores, incrementalStats.Scores)
}

func TestRRCFDetector_ResetReplaysDeterministically(t *testing.T) {
	cfg := DefaultRRCFConfig()
	cfg.NumTrees = 10
	cfg.TreeSize = 8
	cfg.ShingleSize = 2
	detector := NewRRCFDetector(cfg)
	storage := newDetectorTestStorage()

	for ts := int64(1); ts <= 24; ts++ {
		storage.Add(LogMetricsExtractorName, "log.pattern.count", 1, ts, nil)
	}
	for i := 0; i < 10; i++ {
		storage.Add(LogMetricsExtractorName, "log.pattern.count", 1, 25, nil)
	}

	first := detector.Detect(storage, 25)
	firstStats := detector.GetScoreStats()
	detector.Reset()
	second := detector.Detect(storage, 25)
	secondStats := detector.GetScoreStats()

	assert.Equal(t, firstStats.Scores, secondStats.Scores)
	assert.Equal(t, first.Anomalies, second.Anomalies)
}

func TestRRCFDetector_RemoveSeriesReresolvesConfiguredMetrics(t *testing.T) {
	cfg := DefaultRRCFConfig()
	cfg.NumTrees = 5
	cfg.TreeSize = 8
	cfg.ShingleSize = 1
	cfg.Metrics = []RRCFMetricDef{
		{Namespace: "test", Name: "one", Agg: observer.AggregateAverage},
		{Namespace: "test", Name: "two", Agg: observer.AggregateAverage},
	}
	detector := NewRRCFDetector(cfg)
	storage := newDetectorTestStorage()

	for ts := int64(1); ts <= 4; ts++ {
		storage.Add("test", "one", float64(ts), ts, []string{"host:test"})
		storage.Add("test", "two", float64(ts*2), ts, []string{"host:test"})
	}
	detector.Detect(storage, 4)
	replacedRef := detector.resolvedKeys["test|two"]
	require.NotZero(t, replacedRef)
	require.Equal(t, []observer.SeriesRef{replacedRef}, storage.RemoveSeriesByRefs([]observer.SeriesRef{replacedRef}))

	detector.RemoveSeries([]observer.SeriesRef{replacedRef})
	assert.Empty(t, detector.resolvedKeys)
	assert.Zero(t, detector.GetScoreStats().SampleCount)

	for ts := int64(1); ts <= 4; ts++ {
		storage.Add("test", "two", float64(ts*3), ts, []string{"host:test"})
	}
	detector.Detect(storage, 4)

	require.Len(t, detector.resolvedKeys, 2)
	assert.NotEqual(t, replacedRef, detector.resolvedKeys["test|two"])
	assert.Equal(t, 4, detector.GetScoreStats().SampleCount)
}

func TestRRCFDetector_RemoveSeriesPreservesUnrelatedLogState(t *testing.T) {
	detector := NewRRCFDetector(DefaultRRCFConfig())
	storage := newDetectorTestStorage()

	for ts := int64(1); ts <= 4; ts++ {
		storage.Add(LogPatternExtractorName, "log.pattern.one", 1, ts, nil)
		storage.Add(LogPatternExtractorName, "log.pattern.two", 1, ts, nil)
	}
	detector.Detect(storage, 4)
	metas := storage.ListSeries(observer.SeriesFilter{Namespace: LogPatternExtractorName})
	require.Len(t, metas, 2)
	removedRef := metas[0].Ref
	retainedRef := metas[1].Ref
	retainedState := detector.logSeries[retainedRef]
	require.NotNil(t, retainedState)
	require.Equal(t, []observer.SeriesRef{removedRef}, storage.RemoveSeriesByRefs([]observer.SeriesRef{removedRef}))

	detector.RemoveSeries([]observer.SeriesRef{removedRef})

	assert.NotContains(t, detector.logSeries, removedRef)
	assert.Same(t, retainedState, detector.logSeries[retainedRef])
	assert.False(t, detector.logSeriesCached)
}

func TestRRCFShouldEmitAnomalyOnlyOnRisingEdge(t *testing.T) {
	anomalous := false

	assert.True(t, rrcfShouldEmitAnomaly(&anomalous, true, true, 11, 10))
	assert.True(t, anomalous)
	assert.False(t, rrcfShouldEmitAnomaly(&anomalous, true, true, 12, 10))
	assert.True(t, anomalous)
	assert.False(t, rrcfShouldEmitAnomaly(&anomalous, true, true, 9, 10))
	assert.False(t, anomalous)
	assert.True(t, rrcfShouldEmitAnomaly(&anomalous, true, true, 11, 10))
}

func TestRRCFDetector_LogScoreHistoryIsBoundedAndOrdered(t *testing.T) {
	detector := NewRRCFDetector(DefaultRRCFConfig())
	for i := 0; i <= maxRRCFLogScoreHistory; i++ {
		detector.appendLogScore(RRCFScoredPoint{Timestamp: int64(i), Score: float64(i)})
	}

	scores := detector.orderedLogScores()
	require.Len(t, scores, maxRRCFLogScoreHistory)
	assert.Equal(t, int64(1), scores[0].Timestamp)
	assert.Equal(t, int64(maxRRCFLogScoreHistory), scores[len(scores)-1].Timestamp)
}
