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
		NewLogMetricsExtractor(LogMetricsExtractorConfig{
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
		}),
		&ConnectionErrorExtractor{},
	}
}

// BenchmarkLogIngestion_Real_Cardinality measures raw log ingestion cost with
// real extractors (LogMetricsExtractor + ConnectionErrorExtractor) across
// increasing series cardinality.
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
