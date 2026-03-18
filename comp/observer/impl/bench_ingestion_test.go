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

// syntheticRRCFCatalog returns a catalog using defaultCatalog (BOCPD + RRCF + cross_signal)
// with RRCF configured to match the synthetic "ns/metric_N" namespace used in benchmarks.
func syntheticRRCFCatalog(numMetrics int) *componentCatalog {
	cat := defaultCatalog()
	return cat.WithOverride("rrcf", func() any {
		cfg := DefaultRRCFConfig()
		metrics := make([]RRCFMetricDef, numMetrics)
		for i := 0; i < numMetrics; i++ {
			metrics[i] = RRCFMetricDef{Namespace: "ns", Name: fmt.Sprintf("metric_%d", i)}
		}
		cfg.Metrics = metrics
		return NewRRCFDetector(cfg)
	})
}

// bocpdOnlyCatalog returns a catalog with only BOCPD enabled. All other detectors
// and correlators are disabled. Used to isolate BOCPD's CPU cost.
func bocpdOnlyCatalog() *componentCatalog {
	return defaultCatalog().
		WithDefaultEnabled("rrcf", false).
		WithDefaultEnabled("cusum", false).
		WithDefaultEnabled("cross_signal", false).
		WithDefaultEnabled("time_cluster", false).
		WithDefaultEnabled("lead_lag", false).
		WithDefaultEnabled("surprise", false)
}

// rrcfOnlyCatalog returns a catalog with only RRCF enabled, configured for the
// synthetic "ns/metric_N" namespace. All other detectors and correlators are disabled.
func rrcfOnlyCatalog(numMetrics int) *componentCatalog {
	cat := defaultCatalog().
		WithDefaultEnabled("bocpd", false).
		WithDefaultEnabled("cusum", false).
		WithDefaultEnabled("cross_signal", false).
		WithDefaultEnabled("time_cluster", false).
		WithDefaultEnabled("lead_lag", false).
		WithDefaultEnabled("surprise", false)
	return cat.WithOverride("rrcf", func() any {
		cfg := DefaultRRCFConfig()
		metrics := make([]RRCFMetricDef, numMetrics)
		for i := 0; i < numMetrics; i++ {
			metrics[i] = RRCFMetricDef{Namespace: "ns", Name: fmt.Sprintf("metric_%d", i)}
		}
		cfg.Metrics = metrics
		return NewRRCFDetector(cfg)
	})
}

// buildSyntheticStorage creates a storage pre-populated with numSeries series,
// each with numSeconds data points.
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

				// Ingest one second of data across all series.
				for _, o := range obs {
					o.timestamp = int64(i) // unique second per iteration
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
				// Pre-populate storage with the requested history depth.
				storage := buildSyntheticStorage(numSeries, secs)
				e := newEngine(engineConfig{storage: storage})
				b.StartTimer()

				// Ingest one more second beyond existing history.
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

				// Ingest `rate` samples for each series within the same second.
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

// BenchmarkIngestionWithDetection_Cardinality exercises the write path plus
// BOCPD + RRCF detection, ramping cardinality.
func BenchmarkIngestionWithDetection_Cardinality(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			cat := syntheticRRCFCatalog(numSeries)
			rng := rand.New(rand.NewSource(42))
			obs := make([]*metricObs, numSeries)
			for s := 0; s < numSeries; s++ {
				obs[s] = &metricObs{name: fmt.Sprintf("metric_%d", s)}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := newTimeSeriesStorage()
				detectors, correlators, _ := cat.Instantiate(nil)
				e := newEngine(engineConfig{
					storage:     storage,
					detectors:   detectors,
					correlators: correlators,
				})
				e.resetFull()
				b.StartTimer()

				sec := int64(i)
				for _, o := range obs {
					o.value = 100.0 + rng.Float64()*10
					o.timestamp = sec
					reqs := e.IngestMetric("ns", o)
					for _, req := range reqs {
						e.Advance(req.upToSec)
					}
				}
			}
		})
	}
}

// BenchmarkIngestionWithDetection_TimeWindow exercises the write + detection path
// with increasing history depth.
func BenchmarkIngestionWithDetection_TimeWindow(b *testing.B) {
	for _, secs := range []int{100, 600, 1800, 3600} {
		secs := secs
		b.Run(fmt.Sprintf("secs=%d", secs), func(b *testing.B) {
			const numSeries = 50
			cat := syntheticRRCFCatalog(numSeries)
			rng := rand.New(rand.NewSource(42))

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := buildSyntheticStorage(numSeries, secs)
				detectors, correlators, _ := cat.Instantiate(nil)
				e := newEngine(engineConfig{
					storage:     storage,
					detectors:   detectors,
					correlators: correlators,
				})
				e.resetFull()
				b.StartTimer()

				nextSec := int64(secs + i)
				for s := 0; s < numSeries; s++ {
					o := &metricObs{
						name:      fmt.Sprintf("metric_%d", s),
						value:     100.0 + rng.Float64()*10,
						timestamp: nextSec,
					}
					reqs := e.IngestMetric("ns", o)
					for _, req := range reqs {
						e.Advance(req.upToSec)
					}
				}
			}
		})
	}
}

// BenchmarkIngestionWithDetection_SampleRate exercises the write + detection path
// with increasing samples per second per series.
func BenchmarkIngestionWithDetection_SampleRate(b *testing.B) {
	for _, rate := range []int{1, 5, 10, 50} {
		rate := rate
		b.Run(fmt.Sprintf("rate=%d", rate), func(b *testing.B) {
			const numSeries = 50
			cat := syntheticRRCFCatalog(numSeries)
			rng := rand.New(rand.NewSource(42))

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := newTimeSeriesStorage()
				detectors, correlators, _ := cat.Instantiate(nil)
				e := newEngine(engineConfig{
					storage:     storage,
					detectors:   detectors,
					correlators: correlators,
				})
				e.resetFull()
				b.StartTimer()

				sec := int64(i)
				for tick := 0; tick < rate; tick++ {
					for s := 0; s < numSeries; s++ {
						o := &metricObs{
							name:      fmt.Sprintf("metric_%d", s),
							value:     100.0 + rng.Float64()*10,
							timestamp: sec,
						}
						reqs := e.IngestMetric("ns", o)
						for _, req := range reqs {
							e.Advance(req.upToSec)
						}
					}
				}
			}
		})
	}
}

// BenchmarkDetection_Isolated_Cardinality measures steady-state per-advance cost
// for each algorithm in isolation across increasing series cardinality:
//   - bocpd: BOCPD only, no RRCF or correlators
//   - rrcf: RRCF only, no BOCPD or correlators
//   - all: BOCPD + RRCF bundled (baseline for comparison)
//
// Uses the same warm-cursor steady-state pattern as BenchmarkRRCF_SteadyState_v2
// so RRCF has enough history to resolve its feature vectors. Each iteration adds
// 1 new second (untimed + GC) then times a single Advance call.
func BenchmarkDetection_Isolated_Cardinality(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries

		// allDetectorsCatalog has BOCPD + RRCF but no correlators, so the
		// "all" vs "bocpd" and "all" vs "rrcf" deltas reflect only detector
		// cost — not correlator overhead.
		allDetectorsCatalog := rrcfOnlyCatalog(numSeries).WithDefaultEnabled("bocpd", true)

		catalogs := []struct {
			name string
			cat  *componentCatalog
		}{
			{"bocpd", bocpdOnlyCatalog()},
			{"rrcf", rrcfOnlyCatalog(numSeries)},
			{"all", allDetectorsCatalog},
		}

		for _, tc := range catalogs {
			tc := tc
			b.Run(fmt.Sprintf("detector=%s/series=%d", tc.name, numSeries), func(b *testing.B) {
				// 1200s > TreeSize(256) * ShingleSize(4) = 1024s so RRCF is fully
				// populated and evicting on every advance (true steady state).
				storage := buildSyntheticStorage(numSeries, 1200)
				detectors, correlators, _ := tc.cat.Instantiate(nil)
				e := newEngine(engineConfig{
					storage:     storage,
					detectors:   detectors,
					correlators: correlators,
				})
				e.ReplayStoredData()

				rng := rand.New(rand.NewSource(42))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					newSec := int64(1200 + i)

					b.StopTimer()
					for s := 0; s < numSeries; s++ {
						e.Storage().Add("ns", fmt.Sprintf("metric_%d", s), 100.0+rng.Float64()*10, newSec, nil)
					}
					runtime.GC()
					b.StartTimer()

					e.Advance(newSec)
				}
			})
		}
	}
}
