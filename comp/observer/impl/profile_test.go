// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math/rand"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// buildRealisticStorage creates storage mimicking the crash-loop scenario.
func buildRealisticStorage(numMetrics, numSeconds int) *timeSeriesStorage {
	rng := rand.New(rand.NewSource(42))
	storage := newTimeSeriesStorage()

	for sec := int64(0); sec < int64(numSeconds); sec++ {
		for m := 0; m < numMetrics; m++ {
			name := fmt.Sprintf("metric_%d", m)
			value := 100.0 + rng.Float64()*10
			if sec > int64(numSeconds*2/3) {
				value = 200.0 + rng.Float64()*10
			}
			storage.Add("ns", name, value, sec, nil)
		}
	}
	return storage
}

// buildRealisticEngine creates an engine with windowed or unbounded detectors.
func buildRealisticEngine(numMetrics, numSeconds int, windowSec int64) *engine {
	storage := buildRealisticStorage(numMetrics, numSeconds)

	catalog := testbenchCatalog()
	detectors, correlators, _ := catalog.Instantiate(nil)

	// Apply window to all seriesDetectorAdapters.
	if windowSec > 0 {
		for i, d := range detectors {
			if adapter, ok := d.(*seriesDetectorAdapter); ok {
				adapter.windowSec = windowSec
				detectors[i] = adapter
			}
		}
	}

	return newEngine(engineConfig{
		storage:     storage,
		detectors:   detectors,
		correlators: correlators,
	})
}

// BenchmarkReplayStoredData profiles the full replay path (unbounded).
//
//	go test -run=^$ -bench=BenchmarkReplayStoredData -cpuprofile=/tmp/observer_cpu.prof -benchtime=1x ./comp/observer/impl/
//	go tool pprof -http=:8081 /tmp/observer_cpu.prof
func BenchmarkReplayStoredData(b *testing.B) {
	e := buildRealisticEngine(200, 576, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Reset()
		e.ReplayStoredData()
	}
}

// BenchmarkReplayStoredData_Window300 profiles with a 300s window.
func BenchmarkReplayStoredData_Window300(b *testing.B) {
	e := buildRealisticEngine(200, 576, 300)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Reset()
		e.ReplayStoredData()
	}
}

// BenchmarkReplayStoredData_Window60 profiles with a 60s window.
func BenchmarkReplayStoredData_Window60(b *testing.B) {
	e := buildRealisticEngine(200, 576, 60)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Reset()
		e.ReplayStoredData()
	}
}

// BenchmarkReplayStoredData_Small is a smaller variant for quick iteration.
func BenchmarkReplayStoredData_Small(b *testing.B) {
	e := buildRealisticEngine(50, 100, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Reset()
		e.ReplayStoredData()
	}
}

// BenchmarkGetSeriesRange isolates storage read cost.
func BenchmarkGetSeriesRange(b *testing.B) {
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < 576; sec++ {
		storage.Add("ns", "metric_0", 100.0+rand.Float64()*10, sec, nil)
	}
	key := observerdef.SeriesKey{Namespace: "ns", Name: "metric_0"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.GetSeriesRange(key, 0, 576, observerdef.AggregateAverage)
	}
}

// BenchmarkPointCountUpTo isolates the cheap "has new data" check.
func BenchmarkPointCountUpTo(b *testing.B) {
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < 576; sec++ {
		storage.Add("ns", "metric_0", 100.0+rand.Float64()*10, sec, nil)
	}
	key := observerdef.SeriesKey{Namespace: "ns", Name: "metric_0"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storage.PointCountUpTo(key, 300)
	}
}
