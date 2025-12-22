// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

func FuzzTokenizerInvariants(f *testing.F) {
	f.Add([]byte(""), 10)
	f.Add([]byte("abc 123"), 0)
	f.Add([]byte("Jan 123"), 100)
	f.Add([]byte("12-12-12T12:12:12.12T12:12Z123"), 10)

	f.Fuzz(func(t *testing.T, input []byte, maxEvalBytes int) {
		if maxEvalBytes < 0 {
			maxEvalBytes = -maxEvalBytes
		}
		maxEvalBytes = maxEvalBytes % (1 << 12) // bound runtime and allocations

		tokenizer := NewTokenizer(maxEvalBytes)
		ctx := &messageContext{rawMessage: input}
		tokenizer.ProcessAndContinue(ctx)

		maxBytes := len(input)
		if maxEvalBytes < maxBytes {
			maxBytes = maxEvalBytes
		}

		if maxBytes == 0 {
			if len(ctx.tokens) != 0 || len(ctx.tokenIndicies) != 0 {
				t.Fatalf("expected empty tokens/indices for maxBytes=0: tokens=%v indices=%v", ctx.tokens, ctx.tokenIndicies)
			}
			return
		}

		if len(ctx.tokens) == 0 {
			t.Fatalf("expected non-empty tokens for maxBytes=%d input=%q", maxBytes, input[:maxBytes])
		}
		if len(ctx.tokens) != len(ctx.tokenIndicies) {
			t.Fatalf("tokens/indices length mismatch: tokens=%d indices=%d", len(ctx.tokens), len(ctx.tokenIndicies))
		}
		if ctx.tokenIndicies[0] != 0 {
			t.Fatalf("first token index must be 0, got %d (indices=%v)", ctx.tokenIndicies[0], ctx.tokenIndicies)
		}

		prev := -1
		for i, idx := range ctx.tokenIndicies {
			if idx < 0 || idx >= maxBytes {
				t.Fatalf("token index out of range: i=%d idx=%d maxBytes=%d input=%q", i, idx, maxBytes, input[:maxBytes])
			}
			if idx <= prev {
				t.Fatalf("token indices not strictly increasing: i=%d idx=%d prev=%d indices=%v input=%q", i, idx, prev, ctx.tokenIndicies, input[:maxBytes])
			}
			prev = idx
		}

		for _, tok := range ctx.tokens {
			if tok == tokens.End {
				t.Fatalf("token stream must not include End sentinel: %v", ctx.tokens)
			}
			if tok > tokens.End {
				t.Fatalf("token value out of range: %d", tok)
			}
		}

		// Determinism and internal state reuse: tokenizing the same prefix twice must match.
		tokens2, indices2 := tokenizer.tokenize(input[:maxBytes])
		if !equalTokenSlices(ctx.tokens, tokens2) || !equalIntSlices(ctx.tokenIndicies, indices2) {
			t.Fatalf("tokenize mismatch: ctxTokens=%v tokens2=%v ctxIdx=%v idx2=%v input=%q maxBytes=%d", ctx.tokens, tokens2, ctx.tokenIndicies, indices2, input[:maxBytes], maxBytes)
		}
	})
}

func equalTokenSlices(a, b []tokens.Token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

