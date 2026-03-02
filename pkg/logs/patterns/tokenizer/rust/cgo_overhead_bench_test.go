// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !rust_patterns && cgo

// Package rtokenizer provides a CGO overhead microbenchmark.
//
// This file compiles only when rust_patterns is NOT set (Rust lib not available).
// It measures the raw per-call CGO boundary cost using CgoNoop() from cgo_overhead.go.
// Run with: go test -bench=BenchmarkCgo -benchmem ./pkg/logs/patterns/tokenizer/rust/...
//
// Note: Run without rust_patterns tag to execute these benchmarks.
package rtokenizer

import (
	"testing"
)

// BenchmarkCgoNoop measures the raw CGO call overhead (Go→C→Go) with a no-op.
// This establishes the per-call baseline cost of crossing the CGO boundary,
// independent of any Rust tokenization work.
func BenchmarkCgoNoop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		CgoNoop()
	}
}

// BenchmarkCgoNoopBatch1 is an alias for BenchmarkCgoNoop (1 call per iter).
func BenchmarkCgoNoopBatch1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		CgoNoop()
	}
}

// BenchmarkCgoNoopBatch5 measures 5 CGO no-op calls per iteration.
func BenchmarkCgoNoopBatch5(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for j := 0; j < 5; j++ {
			CgoNoop()
		}
	}
}

// BenchmarkCgoNoopBatch10 measures 10 CGO no-op calls per iteration.
func BenchmarkCgoNoopBatch10(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for j := 0; j < 10; j++ {
			CgoNoop()
		}
	}
}

// BenchmarkCgoNoopBatch50 measures 50 CGO no-op calls per iteration.
func BenchmarkCgoNoopBatch50(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for j := 0; j < 50; j++ {
			CgoNoop()
		}
	}
}

// BenchmarkCgoNoopBatch100 measures 100 CGO no-op calls per iteration.
func BenchmarkCgoNoopBatch100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for j := 0; j < 100; j++ {
			CgoNoop()
		}
	}
}
