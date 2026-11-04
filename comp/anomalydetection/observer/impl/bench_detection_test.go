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

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// BenchmarkDetection_SeriesCount measures the steady-state per-advance cost of
// whatever algorithms are currently enabled in defaultCatalog, across increasing
// series count.
//
// Setup: 600s of pre-existing history, detectors warmed via ReplayStoredData.
// Each iteration: add one new second of data (untimed + GC), time one Advance call.
//
// This answers: "at N series, how much CPU does the current detection stack use
// per second of data?" As algorithms are added or removed from the defaults,
// this benchmark automatically reflects the new cost.
func BenchmarkDetection_SeriesCount(b *testing.B) {
	for _, numSeries := range []int{5, 10, 20, 50, 200} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			storage := buildSyntheticStorage(numSeries, 600)
			detectors, correlators, _, _, _ := defaultCatalog().Instantiate(benchmarkSettings)
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

// BenchmarkDetectorAdvance isolates detector cost from storage ingestion and
// correlation. Churn replaces 10% (at least one) of the series before every
// advance, including the production RemoveSeries notification. Replacement
// series have a full warmup window; each iteration keeps cardinality bounded.
func BenchmarkDetectorAdvance(b *testing.B) {
	for _, factory := range []struct {
		name string
		new  func() observer.Detector
	}{
		{"holt", func() observer.Detector { return NewHoltResidualDetector() }},
		{"bocpd", func() observer.Detector { return NewBOCPDDetector(DefaultBOCPDConfig()) }},
		{"tukey", func() observer.Detector { return NewTukeyBiweightDetector() }},
		{"scanmw", func() observer.Detector { return NewScanMWDetector() }},
		{"scanwelch", func() observer.Detector { return NewScanWelchDetector() }},
	} {
		for _, count := range []int{5, 200, 2000} {
			for _, mode := range []string{"idle", "append", "churn"} {
				b.Run(fmt.Sprintf("%s/series=%d/%s", factory.name, count, mode), func(b *testing.B) {
					fixture := newDetectorAdvanceBenchmark(count)
					detector := factory.new()
					detector.Detect(fixture.storage, 120)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						timestamp := int64(120)
						if mode != "idle" {
							b.StopTimer()
							timestamp += int64(i + 1)
							if mode == "churn" {
								removed := fixture.churn(i, timestamp)
								detector.(observer.SeriesRemover).RemoveSeries(removed)
							} else {
								fixture.append(timestamp)
							}
							b.StartTimer()
						}
						result := detector.Detect(fixture.storage, timestamp)
						benchmarkDetectedAnomalies = len(result.Anomalies)
					}
				})
			}
		}
	}
}

var benchmarkDetectedAnomalies int

func BenchmarkDetectorWorkspace(b *testing.B) {
	for _, count := range []int{30, 120} {
		values := make([]float64, count)
		points := make([]observer.Point, count)
		for i := range values {
			values[i] = float64((i * 17) % 23)
			points[i] = observer.Point{Timestamp: int64(i*30 + i%3), Value: values[i]}
		}
		b.Run(fmt.Sprintf("assignRanks/points=%d", count), func(b *testing.B) {
			var workspace scanDetectorWorkspace
			workspace.assignRanks(values)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, benchmarkWorkspaceFloat = workspace.assignRanks(values)
			}
		})
		b.Run(fmt.Sprintf("medianInterval/points=%d", count), func(b *testing.B) {
			var workspace scanDetectorWorkspace
			workspace.medianPointInterval(points)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkWorkspaceInt = workspace.medianPointInterval(points)
			}
		})
	}
}

var (
	benchmarkWorkspaceFloat float64
	benchmarkWorkspaceInt   int64
)

type detectorAdvanceBenchmark struct {
	storage *timeSeriesStorage
	names   []string
	refs    []observer.SeriesRef
}

func newDetectorAdvanceBenchmark(count int) *detectorAdvanceBenchmark {
	config := DefaultStorageConfig()
	config.PointRetentionSecs = 0
	config.MaxPointsPerSeries = 120
	fixture := &detectorAdvanceBenchmark{
		storage: newTimeSeriesStorageWith(config),
		names:   make([]string, count),
		refs:    make([]observer.SeriesRef, count),
	}
	for i := range fixture.names {
		fixture.names[i] = fmt.Sprintf("metric_%d", i)
		fixture.seed(i, 120)
	}
	fixture.storage.Add(observer.TelemetryNamespace, "observer.internal", 1, 120, nil)
	return fixture
}

func (f *detectorAdvanceBenchmark) seed(index int, timestamp int64) {
	for t := timestamp - 119; t <= timestamp; t++ {
		result := f.storage.Add("bench", f.names[index], 100+float64((t+int64(index))%7), t, nil)
		f.refs[index] = result.Ref
	}
}

func (f *detectorAdvanceBenchmark) append(timestamp int64) {
	for i, name := range f.names {
		f.storage.Add("bench", name, 100+float64((timestamp+int64(i))%7), timestamp, nil)
	}
}

func (f *detectorAdvanceBenchmark) churn(iteration int, timestamp int64) []observer.SeriesRef {
	count := max(1, len(f.refs)/10)
	refs := make([]observer.SeriesRef, count)
	for i := range refs {
		refs[i] = f.refs[(iteration*count+i)%len(f.refs)]
	}
	removed := f.storage.RemoveSeriesByRefs(refs)
	for i := range refs {
		f.seed((iteration*count+i)%len(f.refs), timestamp)
	}
	return removed
}

// BenchmarkDetection_AdvanceFrequency measures how batch size — seconds of data
// accumulated before a single Advance call — affects detection cost with the
// current default stack. Series count is fixed at 50.
//
// This answers: "what happens to CPU cost when the scheduler stalls and data
// piles up before being processed?"
func BenchmarkDetection_AdvanceFrequency(b *testing.B) {
	const numSeries = 50

	for _, newSecs := range []int{1, 5, 10, 30} {
		newSecs := newSecs
		b.Run(fmt.Sprintf("newSecs=%d", newSecs), func(b *testing.B) {
			storage := buildSyntheticStorage(numSeries, 600)
			detectors, correlators, _, _, _ := defaultCatalog().Instantiate(benchmarkSettings)
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
