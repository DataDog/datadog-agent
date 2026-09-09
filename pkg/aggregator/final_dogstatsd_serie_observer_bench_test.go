// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const finalDogStatsDSeriesPerOperation = 1_000_000

type noopFinalDogStatsDSerieObserver struct{}

func (noopFinalDogStatsDSerieObserver) ObserveFinalDogStatsDSerie(*metrics.Serie) {}

// BenchmarkTimeSamplerFinalDogStatsDSerieObserverDispatch_1MFinalSeries measures
// final normal-aggregation series dispatch after aggregation filtering.
func BenchmarkTimeSamplerFinalDogStatsDSerieObserverDispatch_1MFinalSeries(b *testing.B) {
	for _, benchmark := range []struct {
		name      string
		observers []FinalDogStatsDSerieObserver
	}{
		{name: "zero-observers"},
		{name: "one-noop-observer", observers: []FinalDogStatsDSerieObserver{noopFinalDogStatsDSerieObserver{}}},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			sampler := &TimeSampler{finalDogStatsDSerieObservers: benchmark.observers}
			serie := &metrics.Serie{}

			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				for i := 0; i < finalDogStatsDSeriesPerOperation; i++ {
					sampler.observeFinalDogStatsDSerie(serie)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(finalDogStatsDSeriesPerOperation), "final-series/op")
		})
	}
}
