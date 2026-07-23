// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pattern

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenizerPromotesUUID(t *testing.T) {
	tokenizer := NewTokenizer(0)
	tokens, indices := tokenizer.Tokenize([]byte("event_id=c05d056c-1c1f-457f-bfd2-f381f7f84e0d done"))

	assert.Equal(t, "CCCCC_CC=UUID CCCC", TokensToString(tokens))
	assert.Equal(t, []int{0, 5, 6, 8, 9, 45, 46}, indices)
}

func TestTokenizerDoesNotPromoteEmbeddedUUID(t *testing.T) {
	tokenizer := NewTokenizer(0)
	tokens, _ := tokenizer.Tokenize([]byte("xc05d056c-1c1f-457f-bfd2-f381f7f84e0d"))

	assert.NotContains(t, tokens, UUID)
}

func TestTokenizerUUIDPreservesIndicesAcrossSegments(t *testing.T) {
	tokenizer := NewTokenizer(0)
	tokens, indices := tokenizer.Tokenize([]byte("a c05d056c-1c1f-457f-bfd2-f381f7f84e0d z"))

	assert.Equal(t, []Token{C1, Space, UUID, Space, Zone}, tokens)
	assert.Equal(t, []int{0, 1, 2, 38, 39}, indices)
}

func TestTokenizerPromotesMultipleAndUppercaseUUIDs(t *testing.T) {
	tokenizer := NewTokenizer(0)
	tokens, indices := tokenizer.Tokenize([]byte(
		"C05D056C-1C1F-457F-BFD2-F381F7F84E0D c05d056c-1c1f-457f-bfd2-f381f7f84e0d",
	))

	assert.Equal(t, []Token{UUID, Space, UUID}, tokens)
	assert.Equal(t, []int{0, 36, 37}, indices)
}

func TestTokenizerDoesNotPromoteMalformedOrTruncatedUUID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
	}{
		{name: "non hex", input: "c05d056c-1c1f-457f-bfd2-f381f7f84e0g"},
		{name: "missing dash", input: "c05d056c-1c1f457f-bfd2-f381f7f84e0d"},
		{name: "word suffix", input: "c05d056c-1c1f-457f-bfd2-f381f7f84e0dx"},
		{name: "truncated by limit", input: "c05d056c-1c1f-457f-bfd2-f381f7f84e0d", maxBytes: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenizer := NewTokenizer(tc.maxBytes)
			tokens, _ := tokenizer.Tokenize([]byte(tc.input))
			assert.NotContains(t, tokens, UUID)
		})
	}
}
