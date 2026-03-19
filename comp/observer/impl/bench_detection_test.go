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

// BenchmarkDetection_Cardinality measures the steady-state per-advance cost of
// whatever algorithms are currently enabled in defaultCatalog, across increasing
// series cardinality.
//
// Setup: 600s of pre-existing history, detectors warmed via ReplayStoredData.
// Each iteration: add one new second of data (untimed + GC), time one Advance call.
//
// This answers: "at N series, how much CPU does the current detection stack use
// per second of data?" As algorithms are added or removed from the defaults,
// this benchmark automatically reflects the new cost.
func BenchmarkDetection_Cardinality(b *testing.B) {
	for _, numSeries := range []int{5, 10, 20, 50, 200, 500} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			storage := buildSyntheticStorage(numSeries, 600)
			detectors, correlators, _, _ := defaultCatalog().Instantiate(ComponentSettings{})
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})
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
				runtime.GC()
				b.StartTimer()

				e.Advance(newSec)
			}
		})
	}
}

// BenchmarkDetection_AdvanceFrequency measures how batch size — seconds of data
// accumulated before a single Advance call — affects detection cost with the
// current default stack. Cardinality is fixed at 50 series.
//
// This answers: "what happens to CPU cost when the scheduler stalls and data
// piles up before being processed?"
func BenchmarkDetection_AdvanceFrequency(b *testing.B) {
	const numSeries = 50

	for _, newSecs := range []int{1, 5, 10, 30} {
		newSecs := newSecs
		b.Run(fmt.Sprintf("newSecs=%d", newSecs), func(b *testing.B) {
			storage := buildSyntheticStorage(numSeries, 600)
			detectors, correlators, _, _ := defaultCatalog().Instantiate(ComponentSettings{})
			e := newEngine(engineConfig{
				storage:     storage,
				detectors:   detectors,
				correlators: correlators,
			})
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
				runtime.GC()
				b.StartTimer()

				e.Advance(latestSec)
			}
		})
	}
}
