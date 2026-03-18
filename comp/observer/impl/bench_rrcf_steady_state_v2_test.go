// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
)

// BenchmarkRRCF_SteadyState_v2_Cardinality measures per-advance cost with GC
// isolation: runtime.GC() is called inside StopTimer after each storage add,
// preventing GC pauses from the setup phase from inflating timed results.
func BenchmarkRRCF_SteadyState_v2_Cardinality(b *testing.B) {
	for _, numMetrics := range []int{5, 10, 20, 50} {
		numMetrics := numMetrics
		b.Run(fmt.Sprintf("metrics=%d", numMetrics), func(b *testing.B) {
			// 1200s > TreeSize(256) * ShingleSize(4) = 1024s, ensuring the forest
			// is fully populated and evicting on every advance (true steady state).
			storage := buildSyntheticStorage(numMetrics, 1200)
			detectors, correlators, _ := syntheticRRCFCatalog(numMetrics).Instantiate(nil)
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})

			// Warm up: advance RRCF cursors to the end of stored history.
			e.ReplayStoredData()

			rng := rand.New(rand.NewSource(42))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				newSec := int64(1200 + i)

				b.StopTimer()
				// Add one new second of data for all N metrics.
				for s := 0; s < numMetrics; s++ {
					value := 100.0 + rng.Float64()*10
					e.Storage().Add("ns", fmt.Sprintf("metric_%d", s), value, newSec, nil)
				}
				// GC isolation: flush allocations from storage adds before timing Advance.
				runtime.GC()
				b.StartTimer()

				e.Advance(newSec)
			}
		})
	}
}

// BenchmarkRRCF_SteadyState_v2_AdvanceFrequency measures how batch size (number
// of new seconds accumulated before a single Advance call) affects per-advance
// cost. Cardinality is fixed at 20 metrics (mid-range). newSecs data points are
// added in the untimed StopTimer phase; a single Advance is timed.
func BenchmarkRRCF_SteadyState_v2_AdvanceFrequency(b *testing.B) {
	const numMetrics = 20

	for _, newSecs := range []int{1, 5, 10, 30} {
		newSecs := newSecs
		b.Run(fmt.Sprintf("newSecs=%d", newSecs), func(b *testing.B) {
			// 1200s > TreeSize(256) * ShingleSize(4) = 1024s, ensuring the forest
			// is fully populated and evicting on every advance (true steady state).
			storage := buildSyntheticStorage(numMetrics, 1200)
			detectors, correlators, _ := syntheticRRCFCatalog(numMetrics).Instantiate(nil)
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})

			// Warm up: advance RRCF cursors to the end of stored history.
			e.ReplayStoredData()

			rng := rand.New(rand.NewSource(42))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// baseSec is the last second already in storage at the start of
				// this iteration. Each iteration advances the window by newSecs.
				baseSec := int64(1200 + i*newSecs)
				latestSec := baseSec + int64(newSecs) - 1

				b.StopTimer()
				// Add newSecs worth of data for all metrics (untimed).
				for sec := baseSec; sec <= latestSec; sec++ {
					for s := 0; s < numMetrics; s++ {
						value := 100.0 + rng.Float64()*10
						e.Storage().Add("ns", fmt.Sprintf("metric_%d", s), value, sec, nil)
					}
				}
				// GC isolation: flush allocations before timing the single Advance.
				runtime.GC()
				b.StartTimer()

				e.Advance(latestSec)
			}
		})
	}
}
