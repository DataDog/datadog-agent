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
// The tokenizer runs on every log line the Agent handles, so its cost is part
// of its contract too:
//
//   - a call reads only the first maxEvalBytes bytes of the line (all of it
//     when maxEvalBytes is 0), in time linear in what it reads; bytes past the
//     window cost nothing;
//   - on a warm Tokenizer (one that has already read a line at least as dense),
//     TokenizeBorrowed, Limit, IsMatch and IsImportant make no heap
//     allocations; Tokenize makes 2, and TokensToString, Clone and Retained
//     make 1 (an empty line makes none);
//   - a Tokenizer with a window of 10 to 64 bytes, like the Logs default of 60,
//     allocates nothing after NewTokenizer; any other Tokenizer's buffers stop
//     growing once it has read its densest line.
//
// perf_contract_test.go enforces the allocation and buffer rules, linear time
// and the window bound in every unit test run, including the race detector and
// coverage runs in CI (the two timing checks skip on Windows, whose clock is
// too coarse for them); reproduce the race runs locally with
// dda inv test --targets=./pkg/logs/tokenizer --race. CPU time needs a
// before-and-after comparison on one machine: after changing this package, run
// perf_compare.sh. It runs BenchmarkConsumer and BenchmarkConsumerMatch,
// each consumer's per-line calls at its production window, against the merge
// base, and fails if one of them, or their geomean, is more than 5% slower, or
// if one allocates more. When a new package starts tokenizing logs, add it to
// consumers in consumer_benchmark_test.go.
package tokenizer
