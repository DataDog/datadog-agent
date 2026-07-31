// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedLogCountExtractor struct{}

func (*fixedLogCountExtractor) Name() string { return "fixed_log_count" }
func (*fixedLogCountExtractor) ProcessLog(log observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	return observerdef.LogMetricsExtractorOutput{Metrics: []observerdef.MetricOutput{{
		Name:    "log.fixed.count",
		Value:   1,
		Tags:    log.Tags(),
		Context: &observerdef.MetricContext{Pattern: "fixed"},
	}}}
}

func TestMaterializedLogCountBucketizerCountsAndZeros(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	b := newMaterializedLogCountBucketizer(LogCountBucketConfig{BucketSeconds: 5, IdleTTLSeconds: 10})
	metric := observerdef.MetricOutput{
		Name:    "log.pattern.count",
		Value:   1,
		Context: &observerdef.MetricContext{Pattern: "request <*>"},
	}
	tags := canonicalizeTags([]string{"service:api"})

	require.True(t, b.observe("logs", metric, 1, tags))
	require.True(t, b.observe("logs", metric, 3, tags))
	b.flush(storage, 15)

	series := storage.GetSeries("logs", "log.pattern.count", tags, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 5, Value: 2},
		{Timestamp: 10, Value: 0},
	}, series.Points)
	meta := storage.ListSeries(observerdef.WorkloadSeriesFilter())
	require.Len(t, meta, 1)
	assert.Equal(t, "request <*>", storage.GetContext(meta[0].Ref).Pattern)
	assert.True(t, storage.SupportsAggregate(meta[0].Ref, observerdef.AggregateAverage))
	assert.False(t, storage.SupportsAggregate(meta[0].Ref, observerdef.AggregateCount))
}

func TestMaterializedLogCountBucketizerStopsAtIdleTTLAndReactivates(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	b := newMaterializedLogCountBucketizer(LogCountBucketConfig{BucketSeconds: 5, IdleTTLSeconds: 10})
	metric := observerdef.MetricOutput{Name: "log.pattern.count", Value: 1}

	require.True(t, b.observe("logs", metric, 1, nil))
	b.flush(storage, 20)
	require.True(t, b.observe("logs", metric, 21, nil))
	b.flush(storage, 35)

	series := storage.GetSeries("logs", "log.pattern.count", nil, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 5, Value: 1},
		{Timestamp: 10, Value: 0},
		{Timestamp: 25, Value: 1},
		{Timestamp: 30, Value: 0},
	}, series.Points)
}

func TestMaterializedLogCountBucketizerRejectsFlushedLateBucket(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	b := newMaterializedLogCountBucketizer(LogCountBucketConfig{BucketSeconds: 5, IdleTTLSeconds: 0})
	metric := observerdef.MetricOutput{Name: "log.pattern.count", Value: 1}

	require.True(t, b.observe("logs", metric, 1, nil))
	b.flush(storage, 5)
	assert.False(t, b.observe("logs", metric, 2, nil))
}

func TestMaterializedLogCountBucketizerOverridesRetentionForLogSeries(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{PointRetentionSecs: 10})
	b := newMaterializedLogCountBucketizer(LogCountBucketConfig{
		BucketSeconds:    5,
		IdleTTLSeconds:   0,
		RetentionSeconds: 60,
	})
	metric := observerdef.MetricOutput{Name: "log.pattern.count", Value: 1}

	require.True(t, b.observe("logs", metric, 1, nil))
	b.flush(storage, 5)
	require.True(t, b.observe("logs", metric, 21, nil))
	b.flush(storage, 25)

	series := storage.GetSeries("logs", "log.pattern.count", nil, observerdef.AggregateAverage)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{
		{Timestamp: 5, Value: 1},
		{Timestamp: 25, Value: 1},
	}, series.Points)
}

func TestMergeLogCountBucketIntervalHandlesOutOfOrderActivity(t *testing.T) {
	intervals := []logCountBucketInterval{{firstEnd: 20, lastEnd: 30}}
	intervals = mergeLogCountBucketInterval(intervals, logCountBucketInterval{firstEnd: 5, lastEnd: 15}, 5)
	intervals = mergeLogCountBucketInterval(intervals, logCountBucketInterval{firstEnd: 40, lastEnd: 45}, 5)

	assert.Equal(t, []logCountBucketInterval{
		{firstEnd: 5, lastEnd: 30},
		{firstEnd: 40, lastEnd: 45},
	}, intervals)
}

func TestEngineMaterializesLogCountBucketsBeforeDetection(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	e := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{&fixedLogCountExtractor{}},
		logCountBuckets: LogCountBucketConfig{
			Enabled:        true,
			BucketSeconds:  5,
			IdleTTLSeconds: 5,
		},
	})

	e.IngestLog("logs", &logObs{timestampMs: 1_000, tags: []string{"service:api"}})
	e.IngestLog("logs", &logObs{timestampMs: 3_000, tags: []string{"service:api"}})
	seriesTags := canonicalizeTags([]string{"observer_source:logs", "service:api"})
	assert.Nil(t, storage.GetSeries(
		"fixed_log_count", "log.fixed.count", seriesTags, observerdef.AggregateAverage,
	))

	e.Advance(5)
	series := storage.GetSeries(
		"fixed_log_count", "log.fixed.count", seriesTags, observerdef.AggregateAverage,
	)
	require.NotNil(t, series)
	assert.Equal(t, []observerdef.Point{{Timestamp: 5, Value: 2}}, series.Points)
}

func TestLogCountBucketConfigFromAgent(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  logs:
    time_buckets:
      enabled: true
      bucket_width: 10s
      idle_ttl: 2m
`)

	got := logCountBucketConfigFromAgent(cfg)
	assert.Equal(t, LogCountBucketConfig{
		Enabled:          true,
		BucketSeconds:    10,
		IdleTTLSeconds:   120,
		RetentionSeconds: 600,
	}, got)
}
