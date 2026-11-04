// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

var (
	benchmarkSeriesMetas []observer.SeriesMeta
	benchmarkSeriesRefs  []observer.SeriesRef
)

func BenchmarkTimeSeriesStorage_ListWorkloadSeries(b *testing.B) {
	for _, numSeries := range []int{200, 10_000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d/ListSeries", numSeries), func(b *testing.B) {
			storage := buildSeriesListingBenchmarkStorage(numSeries)
			filter := observer.WorkloadSeriesFilter()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkSeriesMetas = storage.ListSeries(filter)
			}
		})

		b.Run(fmt.Sprintf("series=%d/ListSeriesRefsInto", numSeries), func(b *testing.B) {
			storage := buildSeriesListingBenchmarkStorage(numSeries)
			filter := observer.WorkloadSeriesFilter()
			refs := make([]observer.SeriesRef, 0, numSeries)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				refs = storage.ListSeriesRefsInto(filter, refs)
			}
			benchmarkSeriesRefs = refs
		})
	}
}

func buildSeriesListingBenchmarkStorage(numSeries int) *timeSeriesStorage {
	storage := newTimeSeriesStorage()
	for i := 0; i < numSeries; i++ {
		tags := []string{
			fmt.Sprintf("container_id:container-%d", i),
			fmt.Sprintf("pod_name:pod-%d", i%100),
			"env:bench",
		}
		storage.Add("bench", fmt.Sprintf("metric_%d", i), float64(i), 1, tags)
	}
	storage.Add(observer.TelemetryNamespace, "observer.internal", 1, 1, nil)
	return storage
}

// BenchmarkDetectorStatusTraversal reproduces the detector selection/status
// path without detector math. Storage ingestion/removal is excluded.
func BenchmarkDetectorStatusTraversal(b *testing.B) {
	for _, count := range []int{5, 200, 2000} {
		for _, mode := range []string{"stable", "churn"} {
			b.Run(fmt.Sprintf("series=%d/%s", count, mode), func(b *testing.B) {
				fixture := newDetectorAdvanceBenchmark(count)
				traverse := func(timestamp int64) {
					total := 0
					forEachSeriesStatus(fixture.storage, observer.WorkloadSeriesFilter(), timestamp, func(ref observer.SeriesRef, status seriesStatus) {
						total += int(ref) + status.pointCount + int(status.writeGeneration)
					})
					benchmarkTraversalChecksum = total
				}
				traverse(120)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					timestamp := int64(120)
					if mode == "churn" {
						b.StopTimer()
						timestamp += int64(i + 1)
						fixture.churn(i, timestamp)
						b.StartTimer()
					}
					traverse(timestamp)
				}
			})
		}
	}
}

var benchmarkTraversalChecksum int

func BenchmarkDetectorStatusSearch(b *testing.B) {
	buckets := make([]pointBucket, 120)
	for i := range buckets {
		buckets[i].timestamp = int64(i + 1)
	}
	for _, timestamp := range []int64{0, 60, 120, 121} {
		b.Run(fmt.Sprintf("timestamp=%d", timestamp), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchmarkTraversalChecksum = searchAfter(buckets, timestamp)
			}
		})
	}
}
