// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"runtime"
	"testing"
)

var benchmarkRetainedStorageSink *timeSeriesStorage

// BenchmarkTimeSeriesStorage_RetainedMemory measures both the retained Go heap
// and the exact backing-array capacity of the per-bucket storage columns.
// Run with -benchtime=1x so each reported sample builds one storage instance.
func BenchmarkTimeSeriesStorage_RetainedMemory(b *testing.B) {
	for _, tc := range []struct {
		name            string
		seriesCount     int
		pointsPerSeries int
	}{
		{name: "dense", seriesCount: 1, pointsPerSeries: 1_000_000},
		{name: "production_window", seriesCount: 5_000, pointsPerSeries: 12},
	} {
		tc := tc
		b.Run(fmt.Sprintf("%s/series=%d/points=%d", tc.name, tc.seriesCount, tc.pointsPerSeries), func(b *testing.B) {
			var retainedBytes uint64
			var columnCapacityBytes uint64

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				benchmarkRetainedStorageSink = nil
				runtime.GC()
				var before runtime.MemStats
				runtime.ReadMemStats(&before)
				b.StartTimer()

				storage := buildRetainedMemoryBenchmarkStorage(tc.seriesCount, tc.pointsPerSeries)

				b.StopTimer()
				benchmarkRetainedStorageSink = storage
				runtime.GC()
				var after runtime.MemStats
				runtime.ReadMemStats(&after)
				if after.HeapAlloc > before.HeapAlloc {
					retainedBytes += after.HeapAlloc - before.HeapAlloc
				}
				columnCapacityBytes += retainedStorageColumnCapacityBytes(storage)
				runtime.KeepAlive(storage)
			}

			b.ReportMetric(float64(retainedBytes)/float64(b.N), "retained-B/op")
			b.ReportMetric(float64(columnCapacityBytes)/float64(b.N), "column-capacity-B/op")
			benchmarkRetainedStorageSink = nil
		})
	}
}

func buildRetainedMemoryBenchmarkStorage(seriesCount, pointsPerSeries int) *timeSeriesStorage {
	storage := newTimeSeriesStorageWith(StorageConfig{})
	for seriesIndex := 0; seriesIndex < seriesCount; seriesIndex++ {
		name := fmt.Sprintf("metric_%d", seriesIndex)
		for pointIndex := 0; pointIndex < pointsPerSeries; pointIndex++ {
			storage.Add("benchmark", name, float64(pointIndex), int64(pointIndex+1), nil)
		}
	}
	return storage
}

func retainedStorageColumnCapacityBytes(storage *timeSeriesStorage) uint64 {
	var total uint64
	for _, stats := range storage.seriesIDStats {
		total += uint64(cap(stats.timestamps)) * uint64(8)
		total += uint64(cap(stats.sums)) * uint64(8)
		total += uint64(cap(stats.counts)) * uint64(8)
	}
	return total
}
