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
// anything from pkg/logs or comp, so any component can depend on it. A
// depguard rule in .golangci.yml enforces this.
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
// until the next call on the same Tokenizer. Copy it (Clone, Retained or Own)
// before keeping it past that point. Own returns an OwnedTokens: an immutable
// copy of the tokens and their start offsets that may be shared between
// goroutines.
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
// TestTokenOrdinalsArePinned fails if an existing value moves. Tokenizer.Contract
// reports ProfileVersion, so consumers can check with Contract.Compatible that
// a stored result matches what they expect.
package tokenizer
