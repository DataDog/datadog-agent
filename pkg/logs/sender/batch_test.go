// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"

	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func createTestBatch() *batch {
	return makeBatch(
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		2,  // maxBatchSize
		10, // maxContentSize
		"test",
		NewMockServerlessMeta(false),
		metrics.NewNoopPipelineMonitor(""),
		metrics.NewNoopPipelineMonitor("").MakeUtilizationMonitor("test", "test"),
		"test",
	)
}

func TestBatchAddMessage(t *testing.T) {
	b := createTestBatch()

	msg := message.NewMessage([]byte("test"), nil, "", 0)
	added, err := b.addMessage(msg)

	assert.True(t, added)
	assert.NoError(t, err)
	assert.False(t, b.buffer.IsEmpty())
}

func TestBatchAddMessageOverflow(t *testing.T) {
	b := createTestBatch()

	// Add messages to fill buffer (maxBatchSize = 2)
	msg1 := message.NewMessage([]byte("a"), nil, "", 0)
	msg2 := message.NewMessage([]byte("b"), nil, "", 0)

	added1, err1 := b.addMessage(msg1)
	added2, err2 := b.addMessage(msg2)

	assert.True(t, added1)
	assert.True(t, added2)
	assert.NoError(t, err1)
	assert.NoError(t, err2)

	// Buffer should be full
	assert.True(t, b.buffer.IsFull())

	// Try to add another message - should not be added
	msg3 := message.NewMessage([]byte("c"), nil, "", 0)
	added3, err3 := b.addMessage(msg3)

	assert.False(t, added3)
	assert.NoError(t, err3)
}

func TestBatchAddMessageTooLarge(t *testing.T) {
	b := makeBatch(
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		2,  // maxBatchSize
		10, // maxContentSize
		"test",
		NewMockServerlessMeta(false),
		metrics.NewNoopPipelineMonitor(""),
		metrics.NewNoopPipelineMonitor("").MakeUtilizationMonitor("test", "test"),
		"test",
	)

	// Send normal message
	normalMsg := message.NewMessage([]byte("test"), nil, "", 0)
	added, err := b.addMessage(normalMsg)
	assert.True(t, added)
	assert.NoError(t, err)

	// Create message larger than maxContentSize (10), should not be added
	largeMsg := message.NewMessage([]byte("this message is too large"), nil, "", 0)
	added, err = b.addMessage(largeMsg)

	assert.False(t, added)
	assert.NoError(t, err)
}

func TestBatchFlushBuffer(t *testing.T) {
	b := createTestBatch()
	output := make(chan *message.Payload, 1)

	// Add a message
	msg := message.NewMessage([]byte("test"), nil, "", 0)
	b.addMessage(msg)

	// Flush the buffer
	b.flushBuffer(output)

	// Should receive payload
	payload := <-output
	assert.Equal(t, 1, len(payload.MessageMetas))
	assert.Equal(t, &msg.MessageMetadata, payload.MessageMetas[0])
	assert.Equal(t, []byte(`[test]`), payload.Encoded)
	assert.Equal(t, "identity", payload.Encoding)

	// Buffer should be empty after flush
	assert.True(t, b.buffer.IsEmpty())
}

func TestBatchFlushEmptyBuffer(t *testing.T) {
	b := createTestBatch()
	output := make(chan *message.Payload, 1)

	// Flush empty buffer
	b.flushBuffer(output)

	// Should not receive anything
	select {
	case <-output:
		assert.Fail(t, "Should not receive payload from empty buffer")
	default:
		// Expected - no payload sent
	}
}

func TestBatchResetBatch(t *testing.T) {
	b := createTestBatch()

	// Add a message
	msg := message.NewMessage([]byte("test"), nil, "", 0)
	b.addMessage(msg)
	assert.False(t, b.buffer.IsEmpty())

	// Reset the batch
	b.resetBatch()

	// Buffer should be empty after reset
	assert.True(t, b.buffer.IsEmpty())

	// Should be able to add messages again
	newMsg := message.NewMessage([]byte("new"), nil, "", 0)
	added, err := b.addMessage(newMsg)
	assert.True(t, added)
	assert.NoError(t, err)
}

func TestBatchProcessMessage(t *testing.T) {
	b := createTestBatch()
	output := make(chan *message.Payload, 10)

	// Process a single message (shouldn't flush yet)
	msg := message.NewMessage([]byte("test"), nil, "", 0)
	b.processMessage(msg, output)

	// Should not flush yet (buffer not full)
	select {
	case <-output:
		assert.Fail(t, "Should not flush with single message")
	default:
		// Expected
	}

	// Add second message to fill buffer (maxBatchSize = 2)
	msg2 := message.NewMessage([]byte("test2"), nil, "", 0)
	b.processMessage(msg2, output)

	// Should flush now
	payload := <-output
	assert.Equal(t, 2, len(payload.MessageMetas))
}

func TestBatchProcessMessageTooLarge(t *testing.T) {
	b := createTestBatch()
	output := make(chan *message.Payload, 10)

	// Process message too large for content size
	largeMsg := message.NewMessage([]byte("this message is way too large for the content size limit"), nil, "", 0)

	b.processMessage(largeMsg, output)

	// Should not receive any payload (message dropped)
	select {
	case <-output:
		assert.Fail(t, "Should not receive payload for dropped message")
	default:
		// Expected - message was dropped
	}
}

func TestBatchSendMessages(t *testing.T) {
	b := createTestBatch()
	output := make(chan *message.Payload, 1)

	// Create message metadata
	msg := message.NewMessage([]byte("test"), nil, "", 0)
	metadata := []*message.MessageMetadata{&msg.MessageMetadata}

	// Simulate serialization
	b.serializer.Serialize(msg, b.writeCounter)

	// Send messages
	b.sendMessages(metadata, output)

	// Should receive payload
	payload := <-output
	assert.Equal(t, metadata, payload.MessageMetas)
	assert.Equal(t, "identity", payload.Encoding)
}
