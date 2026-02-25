// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTokenCapturingHeuristic captures the tokens it receives from the context.
type mockTokenCapturingHeuristic struct {
	capturedTokens  []Token
	capturedIndices []int
}

func (m *mockTokenCapturingHeuristic) ProcessAndContinue(ctx *messageContext) bool {
	m.capturedTokens = ctx.tokens
	m.capturedIndices = ctx.tokenIndicies
	return true
}

// TestLabelerReceivesTokens verifies that heuristics receive the tokens forwarded
// by the Preprocessor (i.e. the Labeler passes them through to the context).
func TestLabelerReceivesTokens(t *testing.T) {
	tok := NewTokenizer(1000)
	content := []byte("2024-01-01 12:00:00 INFO Test message")
	tokens, tokenIndices := tok.Tokenize(content)
	require.NotEmpty(t, tokens, "Tokenizer should produce tokens for this content")

	h := &mockTokenCapturingHeuristic{}
	labeler := NewLabeler([]Heuristic{h}, nil)
	labeler.Label(content, tokens, tokenIndices)

	assert.Equal(t, tokens, h.capturedTokens, "Heuristic should receive the pre-computed tokens")
	assert.Equal(t, tokenIndices, h.capturedIndices, "Heuristic should receive the pre-computed token indices")
}

// TestLabelerEmptyContentProducesNoTokens verifies that when no tokens are passed in,
// the heuristic sees nil/empty tokens.
func TestLabelerEmptyContentProducesNoTokens(t *testing.T) {
	h := &mockTokenCapturingHeuristic{}
	labeler := NewLabeler([]Heuristic{h}, nil)
	labeler.Label([]byte(""), nil, nil)

	assert.Empty(t, h.capturedTokens, "Heuristic should see no tokens when none are passed")
}

// TestLabelConvertsTokensCorrectly verifies that the tokens passed to Label are
// forwarded unchanged to the heuristic context.
func TestLabelConvertsTokensCorrectly(t *testing.T) {
	tok := NewTokenizer(1000)
	content := []byte("2024-01-01")
	tokens, tokenIndices := tok.Tokenize(content)
	require.NotEmpty(t, tokens, "Should have produced tokens for a date-like string")
	// "2024" is a 4-digit run, so the first token should be D4
	assert.Equal(t, D4, tokens[0])

	h := &mockTokenCapturingHeuristic{}
	labeler := NewLabeler([]Heuristic{h}, nil)
	labeler.Label(content, tokens, tokenIndices)

	assert.Equal(t, tokens, h.capturedTokens)
}
