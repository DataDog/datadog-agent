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

// mockTokenCountingHeuristic counts how many times it's called with non-nil tokens
type mockTokenCountingHeuristic struct {
	tokenizeCount int
}

func (m *mockTokenCountingHeuristic) ProcessAndContinue(context *messageContext) bool {
	if len(context.tokens) > 0 {
		m.tokenizeCount++
	}
	return true
}

func TestLabelerReceivesTokensWhenProvided(t *testing.T) {
	mockHeuristic := &mockTokenCountingHeuristic{}
	labeler := NewLabeler([]Heuristic{mockHeuristic}, nil)

	tokens := []Token{
		D1 + 4, // "2024"
		Dash,   // "-"
		D1 + 2, // "01"
		Dash,   // "-"
		D1 + 2, // "01"
		Space,  // " "
	}
	tokenIndices := []int{0, 4, 5, 7, 8, 10}

	labeler.Label([]byte("2024-01-01 12:00:00 INFO Test message"), tokens, tokenIndices)

	assert.Equal(t, 1, mockHeuristic.tokenizeCount, "Heuristic should have seen non-nil tokens")
}

func TestLabelerWorksWithNilTokens(t *testing.T) {
	mockHeuristic := &mockTokenCountingHeuristic{}
	labeler := NewLabeler([]Heuristic{mockHeuristic}, nil)

	labeler.Label([]byte("some log message"), nil, nil)

	assert.Equal(t, 0, mockHeuristic.tokenizeCount, "Heuristic should see no tokens when nil is passed")
}

// mockCapturingHeuristic allows capturing context fields in tests.
type mockCapturingHeuristic struct {
	fn func(*messageContext) bool
}

func (m *mockCapturingHeuristic) ProcessAndContinue(ctx *messageContext) bool {
	return m.fn(ctx)
}

func TestLabelConvertsTokensCorrectly(t *testing.T) {
	var capturedTokens []Token
	wrapper := &mockCapturingHeuristic{fn: func(ctx *messageContext) bool {
		capturedTokens = ctx.tokens
		return true
	}}
	labeler := NewLabeler([]Heuristic{wrapper}, nil)

	tokens := []Token{D1 + 4, Dash, Space}
	tokenIndices := []int{0, 4, 5}

	labeler.Label([]byte("test"), tokens, tokenIndices)

	require.Len(t, capturedTokens, 3, "Should have 3 tokens")
	assert.Equal(t, D1+4, capturedTokens[0])
	assert.Equal(t, Dash, capturedTokens[1])
	assert.Equal(t, Space, capturedTokens[2])
}
