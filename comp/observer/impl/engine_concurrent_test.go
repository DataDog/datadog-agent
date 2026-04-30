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

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// stubContextExtractor is a minimal LogMetricsExtractor that emits one
// metric per ProcessLog with a stable ContextKey so the engine populates
// contextRefs. It also implements ContextProvider so the engine has a
// non-nil entry in contextProviders for enrichAnomaly to look up.
type stubContextExtractor struct {
	name string
}

func (e *stubContextExtractor) Name() string { return e.name }
func (e *stubContextExtractor) ProcessLog(_ observerdef.LogView) observerdef.LogMetricsExtractorOutput {
	return observerdef.LogMetricsExtractorOutput{
		Metrics: []observerdef.MetricOutput{
			{
				Name:       "stub.count",
				Value:      1,
				Tags:       []string{"k:v"},
				ContextKey: "ctx:" + e.name,
			},
		},
	}
}
func (e *stubContextExtractor) GetContextByKey(_ string) (observerdef.MetricContext, bool) {
	return observerdef.MetricContext{Pattern: "stub-pattern"}, true
}

// stubAnomalyDetector emits one anomaly per Detect() with a Source that
// matches the contextRef key the stub extractor writes. This drives
// enrichAnomaly, which reads contextRefs and contextProviders.
type stubAnomalyDetector struct{ extractorName string }

func (d *stubAnomalyDetector) Name() string { return "stub_detector" }
func (d *stubAnomalyDetector) Detect(_ observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{
		Anomalies: []observerdef.Anomaly{{
			Type:         observerdef.AnomalyTypeMetric,
			DetectorName: d.Name(),
			Source: observerdef.SeriesDescriptor{
				Namespace: d.extractorName,
				Name:      "stub.count",
				Tags:      []string{"k:v", "observer_source:source-a"},
			},
			Timestamp: dataTime,
		}},
	}
}

// TestEngine_ConcurrentLogIngestAndAdvance interleaves IngestLog (which
// writes contextRefs) with advanceWithReason (which reads contextRefs
// and contextProviders via enrichAnomaly). The two paths now run on
// different goroutines (run-loop vs scheduler ticker) in production, so
// race-detector coverage of the interleaving is the safety bar.
func TestEngine_ConcurrentLogIngestAndAdvance(t *testing.T) {
	storage := newTimeSeriesStorage()
	extractor := &stubContextExtractor{name: "stub_ext"}
	detector := &stubAnomalyDetector{extractorName: extractor.Name()}
	eng := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
		detectors:  []observerdef.Detector{detector},
		contextProviders: map[string]observerdef.ContextProvider{
			extractor.Name(): extractor,
		},
	})

	const ingestIters = 500
	const advanceIters = 500

	var stop atomic.Bool
	var workers, reader sync.WaitGroup

	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < ingestIters; i++ {
			_, _ = eng.IngestLog("source-a", &logObs{
				content:     []byte("test"),
				timestampMs: int64((i + 1) * 1000),
			})
		}
	}()

	go func() {
		defer workers.Done()
		for i := 0; i < advanceIters; i++ {
			_ = eng.advanceWithReason(int64(i+1), advanceReasonManual)
		}
	}()

	// Reader simulating testbench/state-view callers reading
	// LatestDataTime and LastAnalyzedTime concurrently.
	reader.Add(1)
	go func() {
		defer reader.Done()
		for !stop.Load() {
			_ = eng.latestDataTime.Load()
			_ = eng.lastAnalyzedDataTime.Load()
		}
	}()

	workers.Wait()
	stop.Store(true)
	reader.Wait()

	// Sanity: storage saw the extractor's virtual metrics.
	got := storage.TotalSeriesCount("")
	require.GreaterOrEqual(t, got, 1, "extractor should have written at least one series")
}

// TestStorage_SeriesGenerationUnderConcurrency verifies that the
// SeriesGeneration counter increments exactly once per new series even
// when N writers race to create distinct series.
func TestStorage_SeriesGenerationUnderConcurrency(t *testing.T) {
	const writers = 16
	const seriesPerWriter = 200

	storage := newTimeSeriesStorage()
	startGen := storage.SeriesGeneration()

	var wg sync.WaitGroup
	wg.Add(writers)
	for w := 0; w < writers; w++ {
		go func(writerID int) {
			defer wg.Done()
			for s := 0; s < seriesPerWriter; s++ {
				name := fmt.Sprintf("metric.w%d.s%d", writerID, s)
				storage.Add("ns", name, 1.0, 1, nil)
			}
		}(w)
	}
	wg.Wait()

	expected := uint64(writers * seriesPerWriter)
	delta := storage.SeriesGeneration() - startGen
	assert.Equal(t, expected, delta, "expected one SeriesGeneration tick per new series")
	assert.Equal(t, int(expected), storage.TotalSeriesCount(""))
}
