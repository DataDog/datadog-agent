// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build rust_patterns

// Package rtokenizer provides CGO/FFI benchmarks for the Rust tokenizer.
//
// These benchmarks measure:
//   - Single Tokenize() call overhead (full FFI path: Go→C→Rust→FlatBuffers→Go)
//   - Per-call cost at different "batch sizes" (N sequential Tokenize calls per iteration)
//
// NOTE: The Rust library currently supports only single-log tokenization.
// Each "batch" is implemented as N sequential FFI calls. With a future batch API
// (multiple logs per FFI call), we would expect amortization—these benchmarks
// establish the baseline per-call overhead.
//
// Run with: go test -tags=rust_patterns -bench=BenchmarkTokenize -benchmem ./...
// (Requires libpatterns.dylib on macOS or libpatterns.so on Linux in dev/lib/)
package rtokenizer

import (
	"testing"
)

// Sample log lines for benchmarking (varied complexity)
var benchLogs = []string{
	"2024-01-15 10:30:00 INFO Server started on port 8080",
	"ERROR: Connection timeout after 30s",
	"User admin@example.com logged in from 192.168.1.100",
	"GET /api/v1/users?page=1&limit=10 HTTP/1.1 200",
	"Processing order_id:12345 status:completed amount:99.99",
	"WARN Memory usage: 85%",
	"[2024-01-15T10:30:45+00:00] DEBUG Request processing started",
	`127.0.0.1 - - [10/Feb/2026:12:34:56 +0000] "GET /api/v1/users HTTP/1.1" 200 123 "-" "Mozilla/5.0"`,
	`{"level":"error","msg":"db timeout","duration_ms":3000,"request_id":"r-1","ip":"192.168.1.1"}`,
	`kubelet[123]: I0210 12:34:56.789012 12345 pod_workers.go:191] "SyncPod" pod="default/nginx-12345"`,
}

// BenchmarkTokenizeSingleCall measures the cost of a single Tokenize() call.
// This includes: C.CString, FFI to Rust, FlatBuffers decode, token conversion.
func BenchmarkTokenizeSingleCall(b *testing.B) {
	tokenizer := NewRustTokenizer()
	log := benchLogs[0]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tokenizer.Tokenize(log)
	}
}

// BenchmarkTokenizeBatch1 measures 1 Tokenize call per iteration (baseline).
func BenchmarkTokenizeBatch1(b *testing.B) {
	benchTokenizeBatch(b, 1)
}

// BenchmarkTokenizeBatch5 measures 5 Tokenize calls per iteration.
func BenchmarkTokenizeBatch5(b *testing.B) {
	benchTokenizeBatch(b, 5)
}

// BenchmarkTokenizeBatch10 measures 10 Tokenize calls per iteration.
func BenchmarkTokenizeBatch10(b *testing.B) {
	benchTokenizeBatch(b, 10)
}

// BenchmarkTokenizeBatch50 measures 50 Tokenize calls per iteration.
func BenchmarkTokenizeBatch50(b *testing.B) {
	benchTokenizeBatch(b, 50)
}

// BenchmarkTokenizeBatch100 measures 100 Tokenize calls per iteration.
func BenchmarkTokenizeBatch100(b *testing.B) {
	benchTokenizeBatch(b, 100)
}

func benchTokenizeBatch(b *testing.B, batchSize int) {
	tokenizer := NewRustTokenizer()
	// Cycle through sample logs to avoid trivial optimization
	logIdx := 0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < batchSize; j++ {
			log := benchLogs[logIdx%len(benchLogs)]
			logIdx++
			_, _ = tokenizer.Tokenize(log)
		}
	}
}
