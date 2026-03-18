// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// realExtractors returns the same extractors the live observer registers in NewComponent.
func realExtractors() []observerdef.LogMetricsExtractor {
	return []observerdef.LogMetricsExtractor{
		&LogMetricsExtractor{
			MaxEvalBytes: 4096,
			ExcludeFields: map[string]struct{}{
				"timestamp": {},
				"ts":        {},
				"time":      {},
				"pid":       {},
				"ppid":      {},
				"uid":       {},
				"gid":       {},
			},
		},
		&ConnectionErrorExtractor{},
	}
}

// BenchmarkLogIngestion_Real_Cardinality measures raw log ingestion cost with
// real extractors (LogMetricsExtractor + ConnectionErrorExtractor) across
// increasing series cardinality. Iteration 1 had no extractors; this benchmark
// measures the actual production overhead.
func BenchmarkLogIngestion_Real_Cardinality(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			logs := make([]*logObs, numSeries)
			for s := 0; s < numSeries; s++ {
				logs[s] = &logObs{
					content:     []byte(fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s)),
					status:      "info",
					tags:        []string{fmt.Sprintf("series:%d", s)},
					timestampMs: 0,
				}
			}

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := newTimeSeriesStorage()
				e := newEngine(engineConfig{
					storage:    storage,
					extractors: realExtractors(),
				})
				b.StartTimer()

				tsMs := int64(i) * 1000
				for _, l := range logs {
					l.timestampMs = tsMs
					e.IngestLog("ns", l)
				}
			}
		})
	}
}

// virtualRRCFCatalogFromLogs builds an RRCF catalog that matches the virtual
// metrics produced by ingesting the given logs through realExtractors(). This
// fixes the namespace mismatch in previous iterations where RRCF was configured
// for the "system" namespace and silently matched nothing.
//
// Runs one pass of log ingestion to discover which virtual metric names the
// extractors produce, then configures RRCF to watch them.
func virtualRRCFCatalogFromLogs(logs []*logObs) *componentCatalog {
	probe := newEngine(engineConfig{
		storage:    newTimeSeriesStorage(),
		extractors: realExtractors(),
	})
	for _, l := range logs {
		l2 := *l
		l2.timestampMs = 0
		l2.content = append([]byte(nil), l.content...) // copy to avoid sharing backing array
		probe.IngestLog("ns", &l2)
	}

	allSeries := probe.Storage().AllSeries("ns", AggregateAverage)
	var rrcfMetrics []RRCFMetricDef
	for _, s := range allSeries {
		if len(s.Name) > 9 && s.Name[:9] == "_virtual." {
			rrcfMetrics = append(rrcfMetrics, RRCFMetricDef{
				Namespace: "ns",
				Name:      s.Name,
			})
		}
	}

	cat := defaultCatalog()
	if len(rrcfMetrics) == 0 {
		// No virtual metrics found — disable RRCF rather than silently no-op.
		return cat.WithDefaultEnabled("rrcf", false)
	}
	return cat.WithOverride("rrcf", func() any {
		cfg := DefaultRRCFConfig()
		cfg.Metrics = rrcfMetrics
		return NewRRCFDetector(cfg)
	})
}

// BenchmarkLogIngestionWithDetection_Real_Cardinality measures log ingestion
// with real extractors plus BOCPD + RRCF detection, across increasing series
// cardinality. RRCF is configured to match the virtual metrics the extractors
// actually produce (fixes the previous silent namespace mismatch).
func BenchmarkLogIngestionWithDetection_Real_Cardinality(b *testing.B) {
	for _, numSeries := range []int{50, 200, 500, 2000} {
		numSeries := numSeries
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			logs := make([]*logObs, numSeries)
			for s := 0; s < numSeries; s++ {
				logs[s] = &logObs{
					content: []byte(fmt.Sprintf(`{"msg":"log from series %d","level":"info"}`, s)),
					status:  "info",
					tags:    []string{fmt.Sprintf("series:%d", s)},
				}
			}
			cat := virtualRRCFCatalogFromLogs(logs)

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				storage := newTimeSeriesStorage()
				detectors, correlators, _ := cat.Instantiate(nil)
				e := newEngine(engineConfig{
					storage:     storage,
					extractors:  realExtractors(),
					detectors:   detectors,
					correlators: correlators,
				})
				e.resetFull()
				b.StartTimer()

				tsMs := int64(i) * 1000
				for _, l := range logs {
					l.timestampMs = tsMs
					reqs := e.IngestLog("ns", l)
					for _, req := range reqs {
						e.Advance(req.upToSec)
					}
				}
			}
		})
	}
}
