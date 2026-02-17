// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/tokens"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// mockLineHandler captures messages for testing
type mockLineHandler struct {
	messages []*message.Message
}

func (m *mockLineHandler) process(msg *message.Message) {
	m.messages = append(m.messages, msg)
}

func (m *mockLineHandler) flushChan() <-chan time.Time {
	return nil
}

func (m *mockLineHandler) flush() {}

func TestTokenizingLineHandler_TokenizesMessages(t *testing.T) {
	// Create a mock underlying handler
	mockHandler := &mockLineHandler{
		messages: make([]*message.Message, 0),
	}

	// Create tokenizer with reasonable limit
	tokenizer := tokens.NewTokenizer(1000)

	// Create the tokenizing wrapper
	handler := NewTokenizingLineHandler(tokenizer, mockHandler)

	// Create a test message
	content := []byte("2024-01-01 12:00:00 INFO Starting application")
	msg := message.NewMessage(content, nil, message.StatusInfo, 0)

	// Process the message
	handler.process(msg)

	// Verify message was passed to underlying handler
	require.Len(t, mockHandler.messages, 1)
	processedMsg := mockHandler.messages[0]

	// Verify tokens were populated
	assert.NotNil(t, processedMsg.ParsingExtra.Tokens, "Tokens should be populated")
	assert.NotEmpty(t, processedMsg.ParsingExtra.Tokens, "Tokens should not be empty")

	// Verify the message content is unchanged
	assert.Equal(t, content, processedMsg.GetContent(), "Message content should be unchanged")
}

func TestTokenizingLineHandler_TokenizesDifferentMessages(t *testing.T) {
	mockHandler := &mockLineHandler{
		messages: make([]*message.Message, 0),
	}
	tokenizer := tokens.NewTokenizer(1000)
	handler := NewTokenizingLineHandler(tokenizer, mockHandler)

	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "timestamp log",
			content: "2024-01-01 12:00:00 ERROR Connection failed",
		},
		{
			name:    "json log",
			content: `{"timestamp":"2024-01-01","level":"INFO","msg":"test"}`,
		},
		{
			name:    "plain text",
			content: "Simple log message without structure",
		},
		{
			name:    "multiline stacktrace",
			content: "Exception occurred\n  at com.example.Class.method(Class.java:123)\n  at com.example.Main.main(Main.java:456)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := message.NewMessage([]byte(tc.content), nil, message.StatusInfo, 0)
			handler.process(msg)

			// Find the message in the mock handler
			processedMsg := mockHandler.messages[len(mockHandler.messages)-1]

			// Verify tokens were populated
			assert.NotNil(t, processedMsg.ParsingExtra.Tokens, "Tokens should be populated for: "+tc.name)
			assert.NotEmpty(t, processedMsg.ParsingExtra.Tokens, "Tokens should not be empty for: "+tc.name)
		})
	}
}

func TestTokenizingLineHandler_RespectsMaxInputBytes(t *testing.T) {
	mockHandler := &mockLineHandler{
		messages: make([]*message.Message, 0),
	}

	// Create tokenizer with small limit
	maxBytes := 50
	tokenizer := tokens.NewTokenizer(maxBytes)
	handler := NewTokenizingLineHandler(tokenizer, mockHandler)

	// Create a message longer than the limit
	longContent := make([]byte, 200)
	for i := range longContent {
		longContent[i] = 'a'
	}
	msg := message.NewMessage(longContent, nil, message.StatusInfo, 0)

	// Process the message
	handler.process(msg)

	// Verify tokens were populated (but only for first maxBytes)
	require.Len(t, mockHandler.messages, 1)
	processedMsg := mockHandler.messages[0]
	assert.NotNil(t, processedMsg.ParsingExtra.Tokens, "Tokens should be populated")
	assert.NotEmpty(t, processedMsg.ParsingExtra.Tokens, "Tokens should not be empty")

	// The full content should still be in the message
	assert.Equal(t, longContent, processedMsg.GetContent(), "Full message content should be preserved")
}

func TestTokenizingLineHandler_EmptyMessage(t *testing.T) {
	mockHandler := &mockLineHandler{
		messages: make([]*message.Message, 0),
	}
	tokenizer := tokens.NewTokenizer(1000)
	handler := NewTokenizingLineHandler(tokenizer, mockHandler)

	// Create an empty message
	msg := message.NewMessage([]byte(""), nil, message.StatusInfo, 0)

	// Process the message
	handler.process(msg)

	// Verify message was passed to underlying handler
	require.Len(t, mockHandler.messages, 1)
	processedMsg := mockHandler.messages[0]

	// Empty messages should have no tokens
	assert.Empty(t, processedMsg.ParsingExtra.Tokens, "Empty message should have no tokens")
}

func TestTokenizingLineHandler_FlushChanDelegates(t *testing.T) {
	mockHandler := &mockLineHandler{}
	tokenizer := tokens.NewTokenizer(1000)
	handler := NewTokenizingLineHandler(tokenizer, mockHandler)

	// Verify flushChan returns nil from mock handler
	assert.Nil(t, handler.flushChan(), "flushChan should delegate to underlying handler")
}

func TestTokenizingLineHandler_FlushDelegates(t *testing.T) {
	mockHandler := &mockLineHandler{}
	tokenizer := tokens.NewTokenizer(1000)
	handler := NewTokenizingLineHandler(tokenizer, mockHandler)

	// Verify flush doesn't panic (delegates to underlying handler)
	assert.NotPanics(t, func() {
		handler.flush()
	}, "flush should delegate to underlying handler without panic")
}
