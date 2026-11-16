// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

type recordedLog struct {
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
	Hostname    string   `json:"hostname"`
	TimestampMs int64    `json:"timestamp_ms"`
}

// This opt-in benchmark accepts a pinned JSONL corpus extracted from recordings.
// No customer data or recording fixtures are embedded in the repository.
func readRecordedLogs(tb testing.TB, consume func(recordedLog)) int {
	tb.Helper()
	path := os.Getenv("AAD_LOG_BENCH_CORPUS")
	if path == "" {
		tb.Skip("set AAD_LOG_BENCH_CORPUS to a recorded-log JSONL corpus")
	}
	f, err := os.Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	n := 0
	for {
		var row recordedLog
		err := dec.Decode(&row)
		if err == io.EOF {
			break
		}
		if err != nil {
			tb.Fatal(err)
		}
		consume(row)
		n++
	}
	if n == 0 {
		tb.Fatal("recorded corpus is empty")
	}
	return n
}

func recordedLogEngine(tb testing.TB) *engine {
	tb.Helper()
	_, _, _, extractors, _ := instantiateForTest(tb, defaultCatalog(), ComponentSettings{})
	if os.Getenv("AAD_LOG_BENCH_MODE") == "metrics_only" {
		selected := make([]observerdef.LogMetricsExtractor, 0, 1)
		for _, ex := range extractors {
			if ex.Name() == "log_metrics_extractor" {
				selected = append(selected, ex)
			}
		}
		extractors = selected
	}
	return mustNewEngine(engineConfig{storage: newTimeSeriesStorage(), extractors: extractors})
}

var recordedLogEngineSink *engine

// BenchmarkObserverRecordedLogIngestion measures extraction and bounded storage
// admission. Input decoding and construction are excluded. Detection, reporting,
// and producer queues belong to full-Agent staging measurements.
func BenchmarkObserverRecordedLogIngestion(b *testing.B) {
	b.StopTimer()
	var rows []recordedLog
	readRecordedLogs(b, func(row recordedLog) { rows = append(rows, row) })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recordedLogEngineSink = nil
		runtime.GC()
		e := recordedLogEngine(b)
		b.StartTimer()
		for _, row := range rows {
			e.IngestLog("logs", &logObs{content: row.Content, tags: row.Tags, hostname: row.Hostname, timestampMs: row.TimestampMs})
		}
		b.StopTimer()
		recordedLogEngineSink = e
		b.ReportMetric(float64(e.storage.TotalSeriesCount()), "series")
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(rows)), "ns/log")
	b.ReportMetric(float64(len(rows)), "logs/op")
}

// TestRecordedLogRetainedHeap streams the same corpus, releases decoding
// buffers, forces GC, and keeps only the bounded engine alive. It reports the
// engine-retained heap delta, not process RSS or a full-Agent memory estimate.
func TestRecordedLogRetainedHeap(t *testing.T) {
	if os.Getenv("AAD_LOG_BENCH_CORPUS") == "" {
		t.Skip("set AAD_LOG_BENCH_CORPUS")
	}
	recordedLogEngineSink = nil
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	e := recordedLogEngine(t)
	n := readRecordedLogs(t, func(row recordedLog) {
		e.IngestLog("logs", &logObs{content: row.Content, tags: row.Tags, hostname: row.Hostname, timestampMs: row.TimestampMs})
	})
	recordedLogEngineSink = e
	runtime.GC()
	runtime.ReadMemStats(&after)
	result := map[string]any{"mode": os.Getenv("AAD_LOG_BENCH_MODE"), "logs": n, "series": e.storage.TotalSeriesCount(), "retained_heap_delta_bytes": int64(after.HeapAlloc) - int64(before.HeapAlloc), "heap_alloc_bytes": after.HeapAlloc, "total_allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocated_bytes_per_log_including_decode": float64(after.TotalAlloc-before.TotalAlloc) / float64(n)}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("AAD_RESOURCE %s\n", data)
	runtime.KeepAlive(e)
}
