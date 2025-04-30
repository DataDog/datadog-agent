// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func newTestMessage(content string) *message.Message {
	return message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
}

func TestJSONAggregatorProcess_Complete(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Single complete JSON message
	msg := newTestMessage(`{"key":"value"}`)
	result := aggregator.Process(msg)

	assert.Equal(t, 1, len(result), "Expected one message to be returned")
	assert.Equal(t, []byte(`{"key":"value"}`), result[0].GetContent(), "Content should be unchanged for complete JSON")
}

func TestJSONAggregatorProcess_Incomplete(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Incomplete JSON message
	msg := newTestMessage(`{"key":`)
	result := aggregator.Process(msg)

	assert.Equal(t, 0, len(result), "Expected no messages to be returned for incomplete JSON")
}

func TestJSONAggregatorProcess_MultiPart(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// First part of a JSON message
	msg1 := newTestMessage(`{"key":`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for first incomplete part")

	// Second part completes the JSON
	msg2 := newTestMessage(`"value"}`)
	result = aggregator.Process(msg2)

	assert.Equal(t, 1, len(result), "Expected one message after completion")
	assert.Equal(t, []byte(`{"key":"value"}`), result[0].GetContent(), "Content should be compact JSON")
}

func TestJSONAggregatorProcess_Invalid(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// First part valid
	msg1 := newTestMessage(`{"key":`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for incomplete part")

	// Second part invalid
	msg2 := newTestMessage(`invalid}`)
	result = aggregator.Process(msg2)

	assert.Equal(t, 2, len(result), "Expected original messages to be returned for invalid JSON")
	assert.Equal(t, []byte(`{"key":`), result[0].GetContent(), "First original message should be returned")
	assert.Equal(t, []byte(`invalid}`), result[1].GetContent(), "Second original message should be returned")
}

func TestJSONAggregatorFlush(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Buffer some incomplete JSON
	msg1 := newTestMessage(`{"key":`)
	msg2 := newTestMessage(`"value",`)

	aggregator.Process(msg1)
	aggregator.Process(msg2)

	// Flush and verify all messages are returned
	result := aggregator.Flush()

	assert.Equal(t, 2, len(result), "Expected all buffered messages to be returned")
	assert.Equal(t, []byte(`{"key":`), result[0].GetContent(), "First message content should match")
	assert.Equal(t, []byte(`"value",`), result[1].GetContent(), "Second message content should match")

	// Verify buffer is cleared after flush
	emptyResult := aggregator.Flush()
	assert.Equal(t, 0, len(emptyResult), "Expected empty result after flushing")
}

func TestJSONAggregatorMaxSize(t *testing.T) {
	// Set a small max size to test size limits
	aggregator := NewJSONAggregator(true, 10)

	// First part within size limit
	msg1 := newTestMessage(`{"key":`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for first incomplete part")

	// Second part exceeds size limit
	msg2 := newTestMessage(`"very long value that exceeds the size limit"}`)
	result = aggregator.Process(msg2)

	// Should flush both messages since size limit was exceeded
	assert.Equal(t, 2, len(result), "Expected both messages to be returned when size limit exceeded")
	assert.Equal(t, []byte(`{"key":`), result[0].GetContent(), "First message content should match")
	assert.Equal(t, []byte(`"very long value that exceeds the size limit"}`), result[1].GetContent(), "Second message content should match")

	// Verify buffer is cleared after size limit flush
	emptyResult := aggregator.Flush()
	assert.Equal(t, 0, len(emptyResult), "Expected empty result after size limit flush")
}
