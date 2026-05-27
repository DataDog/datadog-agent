// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build rust_preprocessor && cgo

package preprocessor

import (
	"fmt"
	"testing"
)

func BenchmarkTokenizer_GoVsRust(b *testing.B) {
	windows := []struct {
		name    string
		maxEval int
	}{
		{"Labeler_60B", 60},
		{"Sampler_2048B", 2048},
		{"Unlimited", 0},
	}

	for _, w := range windows {
		b.Run(fmt.Sprintf("Go/%s", w.name), func(b *testing.B) {
			tok := NewTokenizer(w.maxEval)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tok.Tokenize(loadBenchCorpus[i%len(loadBenchCorpus)])
			}
		})
		b.Run(fmt.Sprintf("Rust/%s", w.name), func(b *testing.B) {
			tok := NewRustTokenizer(w.maxEval)
			defer tok.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tok.Tokenize(loadBenchCorpus[i%len(loadBenchCorpus)])
			}
		})
	}
}

// BenchmarkFFIOverhead measures the raw CGo crossing cost with a minimal tokenize call.
func BenchmarkFFIOverhead(b *testing.B) {
	tok := NewRustTokenizer(0)
	defer tok.Close()
	empty := []byte("a")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tok.Tokenize(empty)
	}
}
