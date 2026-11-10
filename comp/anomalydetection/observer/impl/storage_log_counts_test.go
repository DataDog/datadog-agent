// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"testing"
	"unsafe"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

// Materialized reference flush, kept here to compare the complete pipeline
// against the previous storage behavior without a production mode switch.
func flushMaterializedLogCountReference(b *materializedLogCountBucketizer, storage *timeSeriesStorage, upTo int64) {
	if upTo <= b.flushedThrough {
		return
	}
	for key, state := range b.series {
		remaining := state.intervals[:0]
		for _, interval := range state.intervals {
			next := interval.firstEnd
			for next <= interval.lastEnd && next <= upTo {
				result := storage.AddWithKeyAndHostComposite(state.namespace, state.name, state.host, state.values[next], next, state.tags, state.seriesKey, metrics.CountType)
				requireNoReferenceEviction(result)
				if state.hasContext {
					storage.SetContext(result.Ref, state.context)
				}
				state.storageRef = result.Ref
				storage.SetSupportedAggregations(result.Ref, observer.AggregateAverage)
				storage.SetSeriesRetention(result.Ref, b.config.RetentionSeconds)
				storage.SetSeriesActivityTimestamp(result.Ref, state.lastObserved)
				delete(state.values, next)
				next += b.config.BucketSeconds
			}
			if next <= interval.lastEnd {
				interval.firstEnd = next
				remaining = append(remaining, interval)
			}
		}
		state.intervals = remaining
		if len(remaining) == 0 && len(state.values) == 0 {
			delete(b.series, key)
		}
	}
	b.flushedThrough = upTo
}

func requireNoReferenceEviction(result AddResult) {
	if len(result.CapacityEvictedRefs) != 0 {
		panic("reference fixture must not evict series")
	}
}

func TestImplicitLogCountBucketizerParity(t *testing.T) {
	for _, width := range []int64{1, 5, 17} {
		t.Run(strconv.FormatInt(width, 10), func(t *testing.T) {
			cfg := LogCountBucketConfig{Enabled: true, BucketSeconds: width, IdleTTLSeconds: 21, RetentionSeconds: 65}
			denseB, sparseB := newMaterializedLogCountBucketizer(cfg), newMaterializedLogCountBucketizer(cfg)
			dense, sparse := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 24}), newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 24})
			metric := observer.MetricOutput{MetricType: metrics.CountType, Name: "errors", Value: 1}
			for _, at := range []int64{1, 3, 2, 6, 70, 73, 75, 76, 150} {
				left := denseB.observe("logs", metric, "", at, tagset.CompositeTags{}, 42)
				right := sparseB.observe("logs", metric, "", at, tagset.CompositeTags{}, 42)
				require.Equal(t, left, right)
				flushMaterializedLogCountReference(denseB, dense, at-1)
				sparseB.flush(sparse, at-1)
				if len(dense.series) > 0 {
					compareLogCountReaders(t, dense, sparse, 0, at-1)
				}
			}
			flushMaterializedLogCountReference(denseB, dense, 200)
			sparseB.flush(sparse, 200)
			compareLogCountReaders(t, dense, sparse, 0, 200)
		})
	}
}

// appendReferenceLogCount retains the previous representation: one ordinary
// storage point for every completed interval, including zeros. The two paths
// differ only in representation, not scheduling, retention, or detector config.
func appendReferenceLogCount(s *timeSeriesStorage, sparse bool, key uint64, value float64, timestamp int64) observer.SeriesRef {
	width := int64(0)
	if sparse {
		width = 5
	}
	result := s.addWithKeyAndHost("logs", "errors", "", value, timestamp, tagset.CompositeTags{}, key, metrics.CountType, width)
	s.SetSupportedAggregations(result.Ref, observer.AggregateAverage)
	return result.Ref
}

func compareLogCountReaders(t *testing.T, dense, sparse *timeSeriesStorage, ref observer.SeriesRef, timestamp int64) {
	t.Helper()
	require.Equal(t, dense.WriteGeneration(ref), sparse.WriteGeneration(ref))
	require.Equal(t, dense.SeriesGeneration(), sparse.SeriesGeneration())
	require.Equal(t, dense.BulkSeriesStatus([]observer.SeriesRef{ref}, timestamp), sparse.BulkSeriesStatus([]observer.SeriesRef{ref}, timestamp))
	require.Equal(t, dense.ListSeriesMetadata("logs"), sparse.ListSeriesMetadata("logs"))
	require.Equal(t, dense.TotalSampleCount(""), sparse.TotalSampleCount(""))
	require.Equal(t, dense.DataTimestamps(), sparse.DataTimestamps())
	require.Equal(t, dense.MaxTimestamp(), sparse.MaxTimestamp())
	dmin, dmax, dok := dense.TimeBounds()
	smin, smax, sok := sparse.TimeBounds()
	require.Equal(t, []int64{dmin, dmax}, []int64{smin, smax})
	require.Equal(t, dok, sok)
	for _, end := range []int64{0, timestamp - 63, timestamp - 1, timestamp, timestamp + 50} {
		require.Equal(t, dense.PointCountUpTo(ref, end), sparse.PointCountUpTo(ref, end))
		for _, previous := range []int64{end - 1, end - 19} {
			require.Equal(t, dense.FreshSeriesCount("", end, previous), sparse.FreshSeriesCount("", end, previous))
		}
		for _, agg := range []Aggregate{AggregateAverage, AggregateSum, AggregateCount} {
			for _, start := range []int64{0, max(0, end-41), end} {
				if start > end {
					continue
				}
				want := dense.GetSeriesRange(ref, start, end, agg)
				require.Equal(t, want, sparse.GetSeriesRange(ref, start, end, agg))
				require.Equal(t, dense.SumRange(ref, start, end, agg), sparse.SumRange(ref, start, end, agg))
				var got []observer.Point
				require.True(t, sparse.ForEachPoint(ref, start, end, agg, func(_ *observer.Series, point observer.Point) { got = append(got, point) }))
				require.EqualValues(t, len(want.Points), len(got))
				if len(got) > 0 {
					require.Equal(t, want.Points, got)
				}
			}
			for _, n := range []int{1, 7, 24} {
				var want, got []observer.Point
				dense.ForEachLastPoints(ref, end, n, agg, func(_ *observer.Series, point observer.Point) { want = append(want, point) })
				sparse.ForEachLastPoints(ref, end, n, agg, func(_ *observer.Series, point observer.Point) { got = append(got, point) })
				require.Equal(t, want, got)
			}
		}
	}
	require.Equal(t, dense.GetSeriesByNumericID(ref, AggregateAverage), sparse.GetSeriesByNumericID(ref, AggregateAverage))
}

func TestImplicitLogCountReaderParity(t *testing.T) {
	for _, cfg := range []StorageConfig{
		{},
		{PointRetentionSecs: 137},
		{MaxPointsPerSeries: 24},
		{PointRetentionSecs: 83, MaxPointsPerSeries: 24},
	} {
		t.Run(fmt.Sprintf("retention=%d/cap=%d", cfg.PointRetentionSecs, cfg.MaxPointsPerSeries), func(t *testing.T) {
			dense, sparse := newTimeSeriesStorageWith(cfg), newTimeSeriesStorageWith(cfg)
			rng := rand.New(rand.NewSource(19))
			timestamp := int64(100)
			for i := 0; i < 250; i++ {
				timestamp += 5
				if i%47 == 0 {
					// Disjoint coverage and a different bucket anchor after idle.
					timestamp += 17
				}
				value := 0.0
				if rng.Intn(9) == 0 {
					value = float64(1 + rng.Intn(100))
				}
				ref := appendReferenceLogCount(dense, false, 42, value, timestamp)
				require.Equal(t, ref, appendReferenceLogCount(sparse, true, 42, value, timestamp))
				compareLogCountReaders(t, dense, sparse, ref, timestamp)
			}
			ss := sparse.resolveByID(0)
			require.NotNil(t, ss.logCounts)
			for _, bucket := range ss.buckets {
				require.NotZero(t, bucket.sum, "zeros must remain implicit")
			}
			require.Less(t, len(ss.buckets), ss.pointCount())
			require.LessOrEqual(t, len(ss.logCounts.intervals), ss.pointCount())
		})
	}
}

func TestImplicitLogCountZeroOnlyRetention(t *testing.T) {
	cfg := StorageConfig{PointRetentionSecs: 40, MaxPointsPerSeries: 5}
	dense, sparse := newTimeSeriesStorageWith(cfg), newTimeSeriesStorageWith(cfg)
	for i := int64(1); i <= 40; i++ {
		value := 0.0
		if i == 1 {
			value = 3
		}
		appendReferenceLogCount(dense, false, 42, value, i*5)
		appendReferenceLogCount(sparse, true, 42, value, i*5)
	}
	compareLogCountReaders(t, dense, sparse, 0, 200)
	require.Empty(t, sparse.resolveByID(0).buckets)
	require.Equal(t, 6, sparse.PointCountUpTo(0, 200))
	require.Equal(t, int64(40), sparse.WriteGeneration(0))
}

func TestImplicitLogCountSwitchesRepresentation(t *testing.T) {
	dense, sparse := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 4}), newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 4})
	timestamp := int64(0)
	for _, value := range []float64{1, 0, 1, 0, 1} {
		for range 10 {
			timestamp += 5
			appendReferenceLogCount(dense, false, 42, value, timestamp)
			appendReferenceLogCount(sparse, true, 42, value, timestamp)
			compareLogCountReaders(t, dense, sparse, 0, timestamp)
		}
		if value == 0 {
			require.NotNil(t, sparse.resolveByID(0).logCounts)
			require.Nil(t, sparse.resolveByID(0).buckets, "release expired nonzero backing storage")
		} else {
			require.Nil(t, sparse.resolveByID(0).logCounts, "busy series need no interval metadata")
		}
	}
}

func logCountParityDetectors() []func() observer.Detector {
	return []func() observer.Detector{
		func() observer.Detector { return NewBOCPDDetector(DefaultBOCPDConfig()) },
		func() observer.Detector { return NewHoltResidualDetector(DefaultHoltResidualConfig()) },
		func() observer.Detector { return NewTukeyBiweightDetector(DefaultTukeyBiweightConfig()) },
		func() observer.Detector { return NewScanMWDetector(DefaultScanMWConfig()) },
		func() observer.Detector { return NewScanWelchDetector(DefaultScanWelchConfig()) },
	}
}

func TestImplicitLogCountDetectorParity(t *testing.T) {
	for _, makeDetector := range logCountParityDetectors() {
		for _, preloaded := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preloaded=%t", makeDetector().Name(), preloaded), func(t *testing.T) {
				cfg := StorageConfig{}
				if !preloaded {
					cfg.MaxPointsPerSeries, cfg.PointRetentionSecs = 120, 600
				}
				dense, sparse := newTimeSeriesStorageWith(cfg), newTimeSeriesStorageWith(cfg)
				left, right := makeDetector(), makeDetector()
				appendAt := func(i int64) {
					value := 0.0
					if i%10 == 0 || i/100%3 == 1 {
						value = float64(10 + i%7)
					}
					appendReferenceLogCount(dense, false, 42, value, i*5)
					appendReferenceLogCount(sparse, true, 42, value, i*5)
				}
				if preloaded {
					for i := int64(1); i <= 500; i++ {
						appendAt(i)
					}
				}
				for i := int64(1); i <= 500; i++ {
					if !preloaded {
						appendAt(i)
					}
					for _, timestamp := range []int64{i*5 - 1, i * 5, i * 5} {
						want := left.Detect(dense, timestamp, observer.DetectionFilter{})
						got := right.Detect(sparse, timestamp, observer.DetectionFilter{})
						require.Equal(t, want, got, "at time %d", timestamp)
						require.Equal(t, left.Ready(), right.Ready(), "readiness at time %d", timestamp)
					}
				}
				require.True(t, right.Ready())
				// State, including processing cursors and posteriors, must agree even
				// for long stretches that produced no anomaly.
				require.Equal(t, left, right)
			})
		}
	}
}

// Both engines receive real logs through the same extractor. For the reference,
// flush the old dense representation before Advance and temporarily detach its
// already-flushed bucketizer. These fixtures disable baseline/capacity/inactivity
// eviction; dedicated engine tests cover those removal paths.
func advanceMaterializedLogCountReference(e *engine, timestamp int64) advanceResult {
	buckets := e.logCounts
	flushMaterializedLogCountReference(buckets, e.storage, timestamp)
	e.logCounts = nil
	result := e.Advance(timestamp)
	e.logCounts = buckets
	return result
}

func newLogCountParityEngine(t testing.TB) *engine {
	t.Helper()
	e, err := newEngine(engineConfig{
		storage:         newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120, PointRetentionSecs: 600}),
		extractors:      []observer.LogMetricsExtractor{&fixedLogCountExtractor{}},
		detectors:       []detectorRegistration{{detector: NewBOCPDDetector(DefaultBOCPDConfig())}},
		scorer:          newAnomalyScorerBase(DefaultAnomalyScorerConfig()),
		logCountBuckets: LogCountBucketConfig{Enabled: true, BucketSeconds: 5, IdleTTLSeconds: 300, RetentionSeconds: 600},
	})
	require.NoError(t, err)
	return e
}

func TestImplicitLogCountEngineParity(t *testing.T) {
	dense, sparse := newLogCountParityEngine(t), newLogCountParityEngine(t)
	found := 0
	for i := int64(1); i <= 500; i++ {
		logs := 0
		if i%12 == 0 {
			logs = 1
		}
		if i/100%3 == 1 {
			logs = 20
		}
		for n := 0; n < logs; n++ {
			obs := &logObs{timestampMs: (i*5 - 4) * 1000, tags: []string{"service:api"}}
			dense.IngestLog("logs", obs)
			sparse.IngestLog("logs", obs)
		}
		want := advanceMaterializedLogCountReference(dense, i*5)
		got := sparse.Advance(i * 5)
		require.Equal(t, want, got, "at interval %d", i)
		require.Equal(t, dense.scorer.ScoreState(), sparse.scorer.ScoreState())
		found += len(got.anomalies)
	}
	require.Positive(t, found, "fixture must exercise anomaly outputs")
}

func BenchmarkImplicitLogCountEngine(b *testing.B) {
	for _, every := range []int{1, 12} {
		for _, sparse := range []bool{false, true} {
			b.Run(fmt.Sprintf("nonzero_every=%d/sparse=%t", every, sparse), func(b *testing.B) {
				e := newLogCountParityEngine(b)
				logs := make([]logObs, 200)
				for i := range logs {
					logs[i].tags = []string{fmt.Sprintf("service:%d", i)}
				}
				step := func(i int) {
					if i%every == 0 {
						for j := range logs {
							logs[j].timestampMs = int64(i*5+1) * 1000
							e.IngestLog("logs", &logs[j])
						}
					}
					if sparse {
						e.Advance(int64(i*5 + 5))
					} else {
						advanceMaterializedLogCountReference(e, int64(i*5+5))
					}
				}
				for i := 0; i < 240; i++ {
					step(i)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					step(i + 240)
				}
			})
		}
	}
}

func BenchmarkImplicitLogCountStorage(b *testing.B) {
	for _, every := range []int{1, 12} {
		for _, sparse := range []bool{false, true} {
			b.Run(fmt.Sprintf("nonzero_every=%d/sparse=%t", every, sparse), func(b *testing.B) {
				s := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120, PointRetentionSecs: 600})
				for i := int64(1); i <= 120; i++ {
					for key := uint64(1); key <= 200; key++ {
						appendReferenceLogCount(s, sparse, key, 1, i*5)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					value := 0.0
					if i%every == 0 {
						value = 1
					}
					for key := uint64(1); key <= 200; key++ {
						ref := appendReferenceLogCount(s, sparse, key, value, int64(i+121)*5)
						s.ForEachPoint(ref, int64(i+120)*5, int64(i+121)*5, AggregateAverage, func(_ *observer.Series, _ observer.Point) {})
					}
				}
			})
		}
	}
}

// Opt-in retained-heap measurement, separate from allocated bytes/op. It uses
// identical counts and detector inputs, with production-size BOCPD state when
// requested. Run serially so unrelated tests cannot affect the heap snapshots.
func TestImplicitLogCountRetainedHeap(t *testing.T) {
	if os.Getenv("OBSERVER_LOG_COUNT_HEAP") != "1" {
		t.Skip("set OBSERVER_LOG_COUNT_HEAP=1")
	}
	for _, withDetector := range []bool{false, true} {
		for _, sparse := range []bool{false, true} {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			s := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120, PointRetentionSecs: 600})
			for key := uint64(1); key <= 3000; key++ {
				for i := int64(1); i <= 240; i++ {
					value := 0.0
					if i%12 == 0 {
						value = 1
					}
					appendReferenceLogCount(s, sparse, key, value, i*5)
				}
			}
			var detector observer.Detector
			if withDetector {
				detector = NewBOCPDDetector(DefaultBOCPDConfig())
				detector.Detect(s, 1200, observer.DetectionFilter{})
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			var physicalBytes, logicalPoints int
			for _, stats := range s.seriesIDStats {
				physicalBytes += cap(stats.buckets) * int(unsafe.Sizeof(pointBucket{}))
				logicalPoints += stats.pointCount()
			}
			t.Logf("sparse=%t detector=%t retained_bytes=%d numeric_capacity_bytes=%d logical_points=%d", sparse, withDetector, int64(after.HeapAlloc)-int64(before.HeapAlloc), physicalBytes, logicalPoints)
			runtime.KeepAlive(s)
			runtime.KeepAlive(detector)
		}
	}
}
