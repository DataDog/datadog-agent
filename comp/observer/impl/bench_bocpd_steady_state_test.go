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

// BenchmarkBOCPD_SteadyState_Cardinality measures BOCPD's per-advance cost in
// steady state (warm cursors) across increasing series cardinality.
//
// Setup: 600s of pre-existing history, BOCPD warmed up via ReplayStoredData.
// Each iteration: add 1 new second of data (untimed + GC), time one Advance call.
//
// Compare to BenchmarkRRCF_SteadyState_v2_Cardinality to see relative BOCPD vs RRCF cost.
func BenchmarkBOCPD_SteadyState_Cardinality(b *testing.B) {
	for _, numSeries := range []int{5, 10, 20, 50, 200, 500} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			storage := buildSyntheticStorage(numSeries, 600)
			detectors, correlators, _ := bocpdOnlyCatalog().Instantiate(nil)
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})

			// Warm up: advance BOCPD cursors to the end of stored history.
			e.ReplayStoredData()

			rng := rand.New(rand.NewSource(42))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				newSec := int64(600 + i)

				b.StopTimer()
				for s := 0; s < numSeries; s++ {
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

// BenchmarkBOCPD_SteadyState_AdvanceFrequency measures how batch size (seconds of data
// accumulated before a single Advance) affects BOCPD's per-advance cost. Cardinality
// is fixed at 50 series. Compare to BenchmarkRRCF_SteadyState_v2_AdvanceFrequency to
// determine if BOCPD shares RRCF's O(n²) sensitivity to scheduling delays.
func BenchmarkBOCPD_SteadyState_AdvanceFrequency(b *testing.B) {
	const numSeries = 50

	for _, newSecs := range []int{1, 5, 10, 30} {
		newSecs := newSecs
		b.Run(fmt.Sprintf("newSecs=%d", newSecs), func(b *testing.B) {
			storage := buildSyntheticStorage(numSeries, 600)
			detectors, correlators, _ := bocpdOnlyCatalog().Instantiate(nil)
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})

			// Warm up: advance BOCPD cursors to the end of stored history.
			e.ReplayStoredData()

			rng := rand.New(rand.NewSource(42))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				baseSec := int64(600 + i*newSecs)
				latestSec := baseSec + int64(newSecs) - 1

				b.StopTimer()
				for sec := baseSec; sec <= latestSec; sec++ {
					for s := 0; s < numSeries; s++ {
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
