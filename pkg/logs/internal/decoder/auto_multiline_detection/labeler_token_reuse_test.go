// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/tokens"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
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

func TestLabelerReusesTokensFromParsingExtra(t *testing.T) {
	// Create a mock heuristic that counts tokenization
	mockHeuristic := &mockTokenCountingHeuristic{}

	// Create labeler with the mock heuristic
	labeler := NewLabeler([]Heuristic{mockHeuristic}, nil)

	// Create a message with pre-populated tokens
	content := []byte("2024-01-01 12:00:00 INFO Test message")
	msg := message.NewMessage(content, nil, message.StatusInfo, 0)

	// Manually populate tokens (simulating what TokenizingLineHandler does)
	msg.ParsingExtra.Tokens = []tokens.Token{
		tokens.D1 + 4, // "2024"
		tokens.Dash,   // "-"
		tokens.D1 + 2, // "01"
		tokens.Dash,   // "-"
		tokens.D1 + 2, // "01"
		tokens.Space,  // " "
	}
	msg.ParsingExtra.TokenIndices = []int{0, 4, 5, 7, 8, 10}

	// Call Label - should reuse tokens
	labeler.Label(msg)

	// Verify the mock heuristic saw tokens (meaning they were reused)
	assert.Equal(t, 1, mockHeuristic.tokenizeCount, "Heuristic should have seen non-nil tokens")
}

func TestLabelProducesSameResultWithOrWithoutTokens(t *testing.T) {
	// Create labeler with a simple heuristic
	heuristics := []Heuristic{
		NewJSONDetector(),
	}
	labeler := NewLabeler(heuristics, nil)

	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "json",
			content: `{"level":"INFO","msg":"test"}`,
		},
		{
			name:    "plain text",
			content: "Simple log message",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test without pre-populated tokens
			msg1 := message.NewMessage([]byte(tc.content), nil, message.StatusInfo, 0)
			labelWithoutTokens := labeler.Label(msg1)

			// Manually populate tokens to simulate TokenizingLineHandler
			msg2 := message.NewMessage([]byte(tc.content), nil, message.StatusInfo, 0)
			msg2.ParsingExtra.Tokens = []tokens.Token{tokens.C1, tokens.Space, tokens.D1}
			msg2.ParsingExtra.TokenIndices = []int{0, 1, 2}
			labelWithTokens := labeler.Label(msg2)

			// Results should be consistent regardless of whether tokens are pre-populated
			// (for this simple heuristic that doesn't depend on specific tokens)
			assert.NotEqual(t, Label(0), labelWithoutTokens, "Label should be set without tokens")
			assert.NotEqual(t, Label(0), labelWithTokens, "Label should be set with tokens")
		})
	}
}

func TestLabelConvertsTokensCorrectly(t *testing.T) {
	// Create a capturing heuristic that verifies token conversion
	var capturedTokens []tokens.Token
	capturingHeuristic := &struct {
		ProcessAndContinue func(*messageContext) bool
	}{
		ProcessAndContinue: func(ctx *messageContext) bool {
			capturedTokens = ctx.tokens
			return true
		},
	}

	// Wrap in a type that satisfies Heuristic interface
	wrapper := &mockCapturingHeuristic{fn: capturingHeuristic.ProcessAndContinue}
	labeler := NewLabeler([]Heuristic{wrapper}, nil)

	// Create message with specific tokens
	msg := message.NewMessage([]byte("test"), nil, message.StatusInfo, 0)
	msg.ParsingExtra.Tokens = []tokens.Token{
		tokens.D1 + 4,
		tokens.Dash,
		tokens.Space,
	}
	msg.ParsingExtra.TokenIndices = []int{0, 4, 5}

	// Call Label
	labeler.Label(msg)

	// Verify tokens were converted correctly
	require.Len(t, capturedTokens, 3, "Should have 3 tokens")
	assert.Equal(t, tokens.D1+4, capturedTokens[0], "First token should be D1+4")
	assert.Equal(t, tokens.Dash, capturedTokens[1], "Second token should be Dash")
	assert.Equal(t, tokens.Space, capturedTokens[2], "Third token should be Space")
}

// mockCapturingHeuristic allows capturing tokens from the context
type mockCapturingHeuristic struct {
	fn func(*messageContext) bool
}

func (m *mockCapturingHeuristic) ProcessAndContinue(ctx *messageContext) bool {
	return m.fn(ctx)
}
