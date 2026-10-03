// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLimitIsNotPrefixStable pins how Limit(n) on a wide pass differs from
// tokenizing only the first n bytes. Limit keeps every token that starts
// before n, with the shape it has in the full input. Consumers that read a
// narrow window off a wide pass (the labeler off the sampler's pass) see the
// wide-pass shape at the boundary.
func TestLimitIsNotPrefixStable(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		maxBytes   int
		wideLimit  []Token // NewTokenizer(0).TokenizeBorrowed(input).Limit(maxBytes)
		narrowScan []Token // NewTokenizer(maxBytes).Tokenize(input)
	}{
		{
			name:       "digit run crossing the boundary keeps its full length",
			input:      "abc 12345",
			maxBytes:   6,
			wideLimit:  []Token{C3, Space, D5},
			narrowScan: []Token{C3, Space, D2},
		},
		{
			name:       "char run crossing the boundary keeps its full length",
			input:      "12 abcdefgh",
			maxBytes:   5,
			wideLimit:  []Token{D2, Space, C8},
			narrowScan: []Token{D2, Space, C2},
		},
		{
			name:       "IPv4 crossing the boundary stays collapsed",
			input:      "10.0.0.1 up",
			maxBytes:   6,
			wideLimit:  []Token{IPv4},
			narrowScan: []Token{D2, Period, D1, Period, D1},
		},
		{
			name:       "boundary on a run edge with no hybrid token agrees",
			input:      "abc 123 x",
			maxBytes:   7,
			wideLimit:  []Token{C3, Space, D3},
			narrowScan: []Token{C3, Space, D3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(tt.input)

			wide := NewTokenizer(0).TokenizeBorrowed(input).Limit(tt.maxBytes)
			narrow, _ := NewTokenizer(tt.maxBytes).Tokenize(input)

			assert.Equal(t, tt.wideLimit, wide.Clone(), "wide pass + Limit")
			assert.Equal(t, tt.narrowScan, narrow, "narrow pass")
			for _, idx := range wide.Indices() {
				assert.Less(t, idx, tt.maxBytes, "Limit keeps only tokens that start before maxBytes")
			}
		})
	}
}
