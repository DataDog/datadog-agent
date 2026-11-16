// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokenizer turns a log line into a compact sequence of structural
// tokens (character classes, run lengths, a few keywords). The logs
// preprocessor uses it for auto multiline detection, adaptive sampling and
// timestamp detection; tokens describe the shape of a line, not its content.
//
// # Dependencies
//
// This package imports only the Go standard library. It must not import
// anything from pkg/logs or comp, so any package in the Agent's root Go module
// can depend on it. A depguard rule in .golangci.yml enforces this.
//
// # Concurrency
//
// A Tokenizer is not safe for concurrent use: it reuses internal buffers
// across calls. Use one Tokenizer per goroutine.
//
// # Borrowed and owned results
//
// Tokenize returns slices the caller owns and may keep. TokenizeBorrowed
// returns a BorrowedTokens view over the Tokenizer's buffers; it is valid only
// until the next call on the same Tokenizer. Copy it (Clone or Retained)
// before keeping it past that point.
//
// # Token values are append-only
//
// The numeric value of a Token is part of its contract. The adaptive sampler
// hashes raw token bytes into the log_hash tag, and End sizes lookup arrays.
// So:
//
//   - add a new Token immediately before End, and bump ProfileVersion;
//   - never renumber or remove an existing Token;
//   - End is a sentinel and may move; no other value may.
//
// More generally, bump ProfileVersion for any change to Tokenize's output: a
// keyword list edit, a byte-class change, the IPv4 rule or the run-length cap
// change token streams without adding a Token. Renaming a token's
// TokensToString form counts too, because consumers may hash those strings.
//
// TestTokenOrdinalsArePinned fails if an existing value moves, and
// TestProfileVersionMatchesTokenizerOutput, whose corpus produces every Token
// and covers every keyword and byte value, fails if the tokens, their offsets
// or their TokensToString form change without a ProfileVersion bump.
// Consumers that store token streams, or strings made from them, should
// record ProfileVersion with them.
//
// # Performance
//
// Every log line the Agent handles goes through the tokenizer, so a slower
// tokenizer means a slower Agent. Three rules keep it cheap:
//
//   - It reads at most maxEvalBytes bytes of a line (60 for Logs auto
//     multiline, 2048 for the adaptive sampler, 4096 for AAD; 0 means the
//     whole line). A 16 MiB line costs the same as a 60-byte one => long
//     lines can't stall the pipeline.
//   - Once warm, it doesn't allocate. TokenizeBorrowed, Limit, IsMatch and
//     IsImportant make 0 allocations per line; Tokenize makes 2 (tokens and
//     offsets); TokensToString, Clone and Retained make 1. Warm means it has
//     already read a line with at least as many tokens, because the buffers
//     only grow to fit the densest line seen.
//   - With a 10 to 64 byte window, like the Logs default of 60, it never
//     allocates after NewTokenizer, not even on its first line.
//
// perf_contract_test.go checks all three in every unit test job, including
// the race and coverage runs. The checks count allocations, so a slow runner
// can't make them fail. The window check times two calls in the same
// process and skips on Windows, whose clock is too coarse. To run them under
// the race detector locally:
//
//	dda inv test --targets=./pkg/logs/tokenizer --race
//
// Speed is checked differently: CPU time only compares on one machine. After
// changing this package, run
//
//	dda inv logs-tokenizer.bench-compare --targets=pkg/logs/tokenizer --bench='^BenchmarkConsumer' --bench-files=consumer_benchmark_test.go
//
// It benchmarks each consumer's real per-line calls at its real window,
// against the merge base, in about 2 minutes. It checks CPU and memory: it
// fails if one benchmark, or the geomean, is more than 5% slower, if one
// allocates more often, or if one's bytes per call grow by more than 5%. The
// last one catches an allocation that keeps its count but gets bigger, which
// the allocation tests can't see.
//
// Adding a consumer: the Bazel visibility in BUILD.bazel lists every package
// that may import this one, so a new importer doesn't build until it is
// added there. That file is owned by agent-log-pipelines => the change gets
// a Logs review. In the same change, add the new consumer's calls and window
// to consumers in consumer_benchmark_test.go, so the checks and benchmarks
// cover it.
package tokenizer
