# Preprocessor Tokenizer Architecture & Benchmark Baseline

**Date:** 2026-05-18
**Machine:** Apple M1 Max, darwin/arm64
**Branch:** `yoon/tokenizer-in-rust` (based on `main`)
**Purpose:** Baseline before Rust rewrite exploration

See benchmark file: `pkg/logs/internal/decoder/preprocessor/tokenizer_load_benchmark_test.go`

---

## 1. Pipeline Overview

The preprocessor tokenizer is the shared tokenization engine powering two features:
- **Auto-multiline detection** — classifies log lines as startGroup/aggregate/noAggregate
- **Adaptive sampling** — rate-limits logs by structural pattern similarity

Both share a single Tokenizer instance per decoder goroutine.

```
Raw bytes (file/socket/container)
  │
  ▼
Framer → LineParser → Preprocessor.Process()
  │
  ├─ Step 1: JSONAggregator.Process(msg)
  ├─ Step 2: Tokenizer.Tokenize(msg.GetContent())
  │     └─ Window: min(len(content), maxEvalBytes)
  │        labeler=60 bytes, sampler=2048 bytes
  ├─ Step 3: limitTokensToBytes(tokens, indices, labelerMaxBytes)
  ├─ Step 4: Labeler.Label(content, labelTokens, labelIndices)
  │     └─ UserSamples → JSONDetector → TimestampDetector
  │     └─ PatternTable (analytics only)
  ├─ Step 5: Aggregator.Process(msg, label, tokens)
  └─ Step 6: Sampler.Process(msg, tokens)
        └─ AdaptiveSampler: credit-based rate limiting
           isImportant() → linear scan IsMatch() → credit check
```

## 2. Token Type System

`Token` is `type Token byte` — 95 distinct values:
- 1 whitespace, 27 punctuation, 10 digit runs (D1-D10), 10 char runs (C1-C10)
- 5 calendar tokens (Month, Day, Apm, Zone, T)
- 13 severity keywords (Warn, Fatal, Error, Panic, Alert, Severe, Critical, Emergency, Exception, Crash, Failure, Deadlock, Timeout)

Run-length encoding: consecutive same-type chars → single token (capped at 10).
Special token promotion: char runs checked on emit via case-insensitive lookup.

**Rust must replicate this exactly.**

## 3. Key Components

### Tokenizer (tokenizer.go)
- 256-byte lookup table → O(1) per-byte classification
- Not thread-safe (internal buffers reused)
- **2 allocs per call**: result slice copies (lines 219-224)

### TokenGraph (token_graph.go)
- Adjacency matrix built from 73 timestamp formats
- Modified Kadane's algorithm for match probability
- Zero allocs, ~50-90ns per call

### IsMatch (tokenizer.go)
- Positional comparison with early-exit
- Zero allocs, 2-5ns per call
- Early exit: `if match+(count-i-1) < requiredMatches`

### AdaptiveSampler (sampler.go)
- Linear scan calling IsMatch per entry
- Bubble-sort moves hot patterns to front
- Credit-based rate limiting per pattern

## 4. Configuration

| Key | Default | Description |
|---|---|---|
| `auto_multi_line.tokenizer_max_input_bytes` | 60 | Labeler window |
| `experimental_adaptive_sampling.tokenizer_max_input_bytes` | 2048 | Sampler window |
| `experimental_adaptive_sampling.max_patterns` | 1000 | Max tracked patterns |
| `experimental_adaptive_sampling.match_threshold` | 0.9 | Token match threshold |

## 5. Benchmark Results (2026-05-18, Apple M1 Max)

Run benchmarks: `go test ./pkg/logs/internal/decoder/preprocessor/ -run='^$' -bench=Benchmark -benchmem -count=3`

### Tokenizer by window size
| Window | ns/op | B/op | allocs |
|---|---:|---:|---:|
| 60B (labeler) | ~310 | 265 | 2 |
| 2048B (sampler) | ~513 | 476 | 2 |
| Unlimited | ~547 | 476 | 2 |

### Allocation by input size
| Input | ns/op | B/op | allocs |
|---|---:|---:|---:|
| Short (~32B) | ~234 | 168 | 2 |
| Medium (~120B) | ~663 | 480 | 2 |
| Long (~500B) | ~2,042 | 1,584 | 2 |

### Pipeline (noop sampler)
~2,500-6,000 ns/op, 791 B/op, 4 allocs

### Concurrent scaling (tokenizer only)
| Goroutines | ns/op | Speedup |
|---|---:|---:|
| 1 | ~600 | 1x |
| 4 | ~360 | 1.7x |
| 8 | ~310 | 1.9x |
| 16 | ~355 | 1.7x |

## 6. Rust Rewrite Boundary Analysis

### Critical Feasibility Question: FFI Overhead

The Go tokenizer is already very efficient — 256-byte lookup table, no regex, O(n).
CGo FFI overhead is typically 200-500ns per crossing. At ~500ns per tokenize call,
a naive Rust FFI could produce **net-zero or net-negative performance**.

The real win case is **eliminating the 2 result-copy allocations** (168-1,584 B/op).
This requires careful FFI memory design (shared buffer, arena, caller-owned buffer).

Before committing, **run a CGo round-trip microbenchmark** passing ~50 tokens across
the boundary to measure actual FFI cost.

### Option A: Replace Tokenize() only (recommended start)
- Smallest change, token types stay in Go
- Expected: 20-40% on tokenize, ~10-17% on pipeline

### Option B: Replace Tokenize() + IsMatch()
- Single FFI call replaces tokenize + N IsMatch calls
- Expected: 30-50% on sampler path

### Option C: Replace entire Preprocessor
- Maximum gain but huge API surface
- Expected: 50-70% on pipeline, high risk

### Interface needed
```go
type TokenizerBackend interface {
    Tokenize(input []byte) ([]Token, []int)
}
```

## 7. Gotchas for Rust Port

1. Token byte values must match exactly (downstream consumers compare them)
2. Run-length capped at 10 (`"aaaaaaaaaaaaa"` → C10, not C10+C3)
3. Special token promotion is case-insensitive via toUpperLookup
4. Promotion checked on emit (end of run), not during scan
5. One Tokenizer per goroutine (not thread-safe)
6. maxEvalBytes truncation happens before tokenization
7. Output ownership: Go copies internal buffers every call

## 8. Key Files

| File | Purpose |
|---|---|
| `preprocessor/tokens.go` | Token type definitions |
| `preprocessor/tokenizer.go` | Core: Tokenize(), IsMatch(), lookup tables |
| `preprocessor/token_graph.go` | TokenGraph, MatchProbability() |
| `preprocessor/timestamp_detector.go` | 73 timestamp formats, staticTokenGraph |
| `preprocessor/labeler.go` | Heuristic chain |
| `preprocessor/sampler.go` | AdaptiveSampler |
| `preprocessor/preprocessor.go` | Pipeline orchestration |
| `decoder/decoder.go` | Wiring, handler selection |
