// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TestStorageConcurrentAdd_DistinctSeries verifies that many goroutines
// writing to disjoint series in parallel produce the expected per-series
// totals. Disjoint series stress shard fan-out and the cold-path ID
// allocation under contention.
func TestStorageConcurrentAdd_DistinctSeries(t *testing.T) {
	const numWriters = 32
	const seriesPerWriter = 100
	const samplesPerSeries = 50

	storage := newTimeSeriesStorage()

	var wg sync.WaitGroup
	wg.Add(numWriters)
	for w := 0; w < numWriters; w++ {
		go func(writerID int) {
			defer wg.Done()
			for s := 0; s < seriesPerWriter; s++ {
				name := fmt.Sprintf("metric.w%d.s%d", writerID, s)
				tags := []string{fmt.Sprintf("writer:%d", writerID), fmt.Sprintf("series:%d", s)}
				for i := 0; i < samplesPerSeries; i++ {
					storage.Add("ns", name, float64(i+1), int64(i+1), tags)
				}
			}
		}(w)
	}
	wg.Wait()

	got := storage.TotalSeriesCount("")
	require.Equal(t, numWriters*seriesPerWriter, got, "expected one series per (writer, series) pair")

	// Spot-check a few series values: last writer, last series, sample sum
	// should be 1+2+...+samplesPerSeries.
	expectedSum := float64(samplesPerSeries * (samplesPerSeries + 1) / 2)
	for w := 0; w < numWriters; w += 7 {
		for s := 0; s < seriesPerWriter; s += 11 {
			name := fmt.Sprintf("metric.w%d.s%d", w, s)
			tags := []string{fmt.Sprintf("writer:%d", w), fmt.Sprintf("series:%d", s)}
			series := storage.GetSeries("ns", name, tags, AggregateSum)
			require.NotNilf(t, series, "missing series w=%d s=%d", w, s)
			var sum float64
			for _, p := range series.Points {
				sum += p.Value
			}
			assert.Equalf(t, expectedSum, sum, "wrong sum for w=%d s=%d", w, s)
		}
	}
}

// TestStorageConcurrentAdd_SharedSeries verifies that many goroutines
// writing to the SAME series produce the correct merged total. This
// stresses the shard write lock — every write hashes to one shard and
// must serialize, but the result must still be consistent.
func TestStorageConcurrentAdd_SharedSeries(t *testing.T) {
	const numWriters = 16
	const samplesPerWriter = 1000

	storage := newTimeSeriesStorage()
	tags := []string{"shared:true"}

	var wg sync.WaitGroup
	wg.Add(numWriters)
	for w := 0; w < numWriters; w++ {
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < samplesPerWriter; i++ {
				// All writers target the same (namespace, name, tags)
				// at the same timestamp so the bucket update path
				// is exercised under contention.
				storage.Add("ns", "metric.shared", 1.0, 100, tags)
			}
		}(w)
	}
	wg.Wait()

	require.Equal(t, 1, storage.TotalSeriesCount(""), "shared series must coalesce into a single seriesStats")

	series := storage.GetSeries("ns", "metric.shared", tags, AggregateSum)
	require.NotNil(t, series)
	require.Len(t, series.Points, 1)

	// Sum aggregate should be (1.0 * total samples).
	totalSamples := numWriters * samplesPerWriter
	assert.Equal(t, float64(totalSamples), series.Points[0].Value)

	// And the count aggregate should reflect every individual sample.
	cseries := storage.GetSeries("ns", "metric.shared", tags, AggregateCount)
	require.NotNil(t, cseries)
	require.Len(t, cseries.Points, 1)
	assert.Equal(t, float64(totalSamples), cseries.Points[0].Value)
}

// TestStorageConcurrentReadDuringWrite exercises the read path while writes
// are in flight. Readers must never observe torn state: point counts must
// be non-negative and writeGeneration must be monotonic.
func TestStorageConcurrentReadDuringWrite(t *testing.T) {
	const numWriters = 8
	const samplesPerWriter = 5000

	storage := newTimeSeriesStorage()
	tags := []string{"k:v"}

	// Pre-create the series so the reader always has a ref to look up.
	storage.Add("ns", "metric.warm", 1.0, 1, tags)
	metas := storage.ListSeries(observer.SeriesFilter{Namespace: "ns"})
	require.Len(t, metas, 1)
	ref := metas[0].Ref

	var stop atomic.Bool
	var readerErrors atomic.Int64

	var writers, reader sync.WaitGroup
	reader.Add(1)
	go func() {
		defer reader.Done()
		var lastGen int64
		for !stop.Load() {
			gen := storage.WriteGeneration(ref)
			pc := storage.PointCount(ref)
			if pc < 0 || gen < lastGen {
				readerErrors.Add(1)
			}
			lastGen = gen
		}
	}()

	writers.Add(numWriters)
	for w := 0; w < numWriters; w++ {
		go func() {
			defer writers.Done()
			for i := 0; i < samplesPerWriter; i++ {
				ts := int64(1 + (i % 100)) // small range so bucket merges dominate
				storage.Add("ns", "metric.warm", float64(i), ts, tags)
			}
		}()
	}

	writers.Wait()
	stop.Store(true)
	reader.Wait()

	expected := int64(1 + numWriters*samplesPerWriter)
	assert.Equal(t, int64(0), readerErrors.Load(), "reader observed inconsistent state")
	assert.Equal(t, expected, storage.WriteGeneration(ref), "expected one writeGeneration tick per Add")
}
