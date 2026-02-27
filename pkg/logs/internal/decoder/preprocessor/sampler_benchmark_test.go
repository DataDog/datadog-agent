// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package preprocessor

import (
	"math"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Pre-tokenized log patterns used across sampler benchmarks.
// Using a package-level tokenizer so token slices are computed once.
var (
	benchTok = NewTokenizer(0)

	// A realistic INFO log pattern
	benchTokensINFO, _ = benchTok.Tokenize([]byte("2024-01-15 10:30:45.123 INFO [service-a] Request processed user_id=12345 duration_ms=42"))
	// Patterns that won't match benchTokensINFO at 0.75 threshold
	benchFillPatterns     = makeFillPatterns(100)
	benchFillPatterns1000 = makeFillPatterns(1000)
)

// makeFillPatterns generates n distinct token slices that won't match benchTokensINFO.
func makeFillPatterns(n int) [][]Token {
	tok := NewTokenizer(0)
	templates := []string{
		"metric cpu_usage=45.67 host=web-01 env=prod ts=1234567890",
		"audit user=admin action=delete resource=record id=9999 result=ok",
		"health status=ok latency_p99=12ms connections=128 workers=16",
		"cache hit key=user:5678 ttl=300 size=1024 store=redis",
		"http 192.168.1.100 GET /api/v2/items?page=2 200 512 12ms",
		"deploy version=2.3.1 env=staging region=us-east-1 ok=true",
		"queue enqueue job=email_send priority=5 delay=0 id=abc123",
		"storage read path=/data/shard-4/chunk-17 bytes=65536 ms=3",
		"auth token=eyJhbGciOiJSUzI1NiJ9 user=42 scope=read exp=3600",
		"grpc method=GetUser status=OK latency=5ms peer=10.0.0.1",
	}
	patterns := make([][]Token, n)
	for i := range n {
		tokens, _ := tok.Tokenize([]byte(templates[i%len(templates)]))
		// Make a copy so each pattern is distinct in memory
		cp := make([]Token, len(tokens))
		copy(cp, tokens)
		patterns[i] = cp
	}
	return patterns
}

func benchMsg() *message.Message {
	return message.NewMessage([]byte("2024-01-15 10:30:45.123 INFO [service-a] Request processed user_id=12345"), nil, message.StatusInfo, 0)
}

// newBenchSampler creates an AdaptiveSampler with effectively unlimited burst so
// the credits never run out, keeping the benchmark focused on lookup cost.
func newBenchSampler(maxPatterns int) *AdaptiveSampler {
	return NewAdaptiveSampler(AdaptiveSamplerConfig{
		MaxPatterns:    maxPatterns,
		RateLimit:      1e9, // refills instantly at benchmark timescales
		BurstSize:      math.MaxFloat64 / 2,
		MatchThreshold: 0.75,
	}, "bench")
}

// prefillSampler loads the sampler's entries directly, bypassing Process, so we can
// precisely control what is in the table without consuming benchmark time.
// All prefilled entries get matchCount=1 so they form a trivially valid heap;
// heapify() is called at the end to ensure the invariant holds.
func prefillSampler(s *AdaptiveSampler, patterns [][]Token, count int) {
	now := time.Now()
	for i := range count {
		s.entries = append(s.entries, samplerEntry{
			tokens:     patterns[i%len(patterns)],
			credits:    s.config.BurstSize,
			lastSeen:   now,
			matchCount: 1,
		})
	}
	s.heapify()
}

// --- Baselines ---

// BenchmarkSampler_Noop measures the overhead of the no-op pass-through sampler.
func BenchmarkSampler_Noop(b *testing.B) {
	s := NewNoopSampler()
	msg := benchMsg()
	tokens := benchTokensINFO
	b.ResetTimer()
	for range b.N {
		s.Process(msg, tokens)
	}
}

// --- Adaptive sampler: match found ---

// BenchmarkSampler_Adaptive_Match_P10_First is the best case: the matching pattern
// is the first entry in a 10-entry table (scan exits on the first comparison).
func BenchmarkSampler_Adaptive_Match_P10_First(b *testing.B) {
	s := newBenchSampler(50)
	// Matching pattern is at index 0.
	s.entries = append(s.entries, samplerEntry{tokens: benchTokensINFO, credits: s.config.BurstSize, lastSeen: time.Now()})
	prefillSampler(s, benchFillPatterns, 9)
	msg := benchMsg()
	b.ResetTimer()
	for range b.N {
		s.Process(msg, benchTokensINFO)
	}
}

// BenchmarkSampler_Adaptive_Match_P10_Last is the worst case for a 10-entry table:
// the matching pattern is at the last index (full scan required).
func BenchmarkSampler_Adaptive_Match_P10_Last(b *testing.B) {
	s := newBenchSampler(50)
	prefillSampler(s, benchFillPatterns, 9)
	s.entries = append(s.entries, samplerEntry{tokens: benchTokensINFO, credits: s.config.BurstSize, lastSeen: time.Now()})
	msg := benchMsg()
	b.ResetTimer()
	for range b.N {
		s.Process(msg, benchTokensINFO)
	}
}

// BenchmarkSampler_Adaptive_Match_P100_Last is the worst case for a full 100-entry table.
func BenchmarkSampler_Adaptive_Match_P100_Last(b *testing.B) {
	s := newBenchSampler(200)
	prefillSampler(s, benchFillPatterns, 99)
	s.entries = append(s.entries, samplerEntry{tokens: benchTokensINFO, credits: s.config.BurstSize, lastSeen: time.Now()})
	msg := benchMsg()
	b.ResetTimer()
	for range b.N {
		s.Process(msg, benchTokensINFO)
	}
}

// --- Adaptive sampler: no match (new pattern) ---

// BenchmarkSampler_Adaptive_NewPattern_TableNotFull measures the cost when a new
// pattern arrives and there is still room in the table (no eviction needed).
func BenchmarkSampler_Adaptive_NewPattern_TableNotFull(b *testing.B) {
	msg := benchMsg()
	// Re-create the sampler each iteration so the table never fills.
	b.ResetTimer()
	for range b.N {
		s := newBenchSampler(200)
		prefillSampler(s, benchFillPatterns, 10)
		s.Process(msg, benchTokensINFO)
	}
}

// BenchmarkSampler_Adaptive_NewPattern_P100_EvictFreq measures the worst case for
// a 100-entry table: a new pattern arrives when the table is full (full scan + evict
// least-frequent leaf + heap fixup).
func BenchmarkSampler_Adaptive_NewPattern_P100_EvictFreq(b *testing.B) {
	msg := benchMsg()
	b.ResetTimer()
	for range b.N {
		s := newBenchSampler(100)
		prefillSampler(s, benchFillPatterns, 100)
		s.Process(msg, benchTokensINFO)
	}
}

// BenchmarkSampler_Adaptive_Match_P1000_Last is the worst-case scan for a 1000-entry
// table on the first iteration; the heap converges toward best-case as the hot
// pattern accumulates matchCount and rises to the top across iterations.
func BenchmarkSampler_Adaptive_Match_P1000_Last(b *testing.B) {
	s := newBenchSampler(2000)
	prefillSampler(s, benchFillPatterns1000, 999)
	s.entries = append(s.entries, samplerEntry{tokens: benchTokensINFO, credits: s.config.BurstSize, lastSeen: time.Now(), matchCount: 1})
	s.heapify()
	msg := benchMsg()
	b.ResetTimer()
	for range b.N {
		s.Process(msg, benchTokensINFO)
	}
}

// BenchmarkSampler_Adaptive_NewPattern_P1000_EvictFreq measures the cost when a new
// pattern arrives into a full 1000-entry table (full scan + evict least-frequent leaf).
func BenchmarkSampler_Adaptive_NewPattern_P1000_EvictFreq(b *testing.B) {
	msg := benchMsg()
	b.ResetTimer()
	for range b.N {
		s := newBenchSampler(1000)
		prefillSampler(s, benchFillPatterns1000, 1000)
		s.Process(msg, benchTokensINFO)
	}
}
