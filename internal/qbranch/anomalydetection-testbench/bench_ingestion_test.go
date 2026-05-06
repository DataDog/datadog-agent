// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"math/rand"
	"testing"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
)

// buildSyntheticStorage creates a storage pre-populated with numSeries series,
// each with numSeconds data points. The last third of the data has a step-change
// to give detectors a realistic signal shape during warm-up.
func buildSyntheticStorage(numSeries, numSeconds int) *observerimpl.TimeSeriesStorage {
	rng := rand.New(rand.NewSource(42))
	storage := observerimpl.NewTimeSeriesStorage()
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

// BenchmarkIngestion_SeriesCount ramps the number of series, measuring raw write cost.
func BenchmarkIngestion_SeriesCount(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			rng := rand.New(rand.NewSource(42))
			type metric struct {
				name  string
				value float64
			}
			metrics := make([]metric, numSeries)
			for s := 0; s < numSeries; s++ {
				metrics[s] = metric{
					name:  fmt.Sprintf("metric_%d", s),
					value: 100.0 + rng.Float64()*10,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := observerimpl.NewTimeSeriesStorage()
				b.StartTimer()

				ts := int64(i)
				for _, m := range metrics {
					storage.Add("ns", m.name, m.value, ts, nil)
				}
			}
		})
	}
}
