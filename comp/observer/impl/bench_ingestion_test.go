// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math/rand"
	"testing"
)

// buildSyntheticStorage creates a storage pre-populated with numSeries series,
// each with numSeconds data points. The last third of the data has a step-change
// to give detectors a realistic signal shape during warm-up.
func buildSyntheticStorage(numSeries, numSeconds int) *timeSeriesStorage {
	rng := rand.New(rand.NewSource(42))
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < int64(numSeconds); sec++ {
		for s := 0; s < numSeries; s++ {
			name := fmt.Sprintf("metric_%d", s)
			value := 100.0 + rng.Float64()*10
			if sec > int64(numSeconds*2/3) {
				value = 200.0 + rng.Float64()*10
			}
			storage.Add("ns", name, value, sec, nil)
		}
	}
	return storage
}

// BenchmarkIngestion_Cardinality ramps the number of series, measuring raw write cost.
func BenchmarkIngestion_Cardinality(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			rng := rand.New(rand.NewSource(42))
			obs := make([]*metricObs, numSeries)
			for s := 0; s < numSeries; s++ {
				obs[s] = &metricObs{
					name:      fmt.Sprintf("metric_%d", s),
					value:     100.0 + rng.Float64()*10,
					timestamp: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{storage: storage})
				b.StartTimer()

				for _, o := range obs {
					o.timestamp = int64(i)
					e.IngestMetric("ns", o)
				}
			}
		})
	}
}

// BenchmarkIngestion_TimeWindow ramps the pre-existing history depth, measuring
// how storage size affects write-path cost.
func BenchmarkIngestion_TimeWindow(b *testing.B) {
	for _, secs := range []int{100, 600, 1800, 3600} {
		secs := secs
		b.Run(fmt.Sprintf("secs=%d", secs), func(b *testing.B) {
			const numSeries = 50
			rng := rand.New(rand.NewSource(42))

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := buildSyntheticStorage(numSeries, secs)
				e := newEngine(engineConfig{storage: storage})
				b.StartTimer()

				nextSec := int64(secs + i)
				for s := 0; s < numSeries; s++ {
					e.IngestMetric("ns", &metricObs{
						name:      fmt.Sprintf("metric_%d", s),
						value:     100.0 + rng.Float64()*10,
						timestamp: nextSec,
					})
				}
			}
		})
	}
}

// BenchmarkIngestion_SampleRate ramps intra-second samples per series, measuring
// the storage merge overhead when multiple samples land in the same second.
func BenchmarkIngestion_SampleRate(b *testing.B) {
	for _, rate := range []int{1, 5, 10, 50} {
		rate := rate
		b.Run(fmt.Sprintf("rate=%d", rate), func(b *testing.B) {
			const numSeries = 50
			rng := rand.New(rand.NewSource(42))

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{storage: storage})
				b.StartTimer()

				sec := int64(i)
				for tick := 0; tick < rate; tick++ {
					for s := 0; s < numSeries; s++ {
						e.IngestMetric("ns", &metricObs{
							name:      fmt.Sprintf("metric_%d", s),
							value:     100.0 + rng.Float64()*10,
							timestamp: sec,
						})
					}
				}
			}
		})
	}
}
