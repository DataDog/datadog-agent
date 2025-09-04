// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package grpc

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Helper to create test StatefulMessage with Datum
func createTestStatefulMessage(content string) *message.StatefulMessage {
	msg := message.NewMessage([]byte(content), nil, "", 0)
	msg.MessageMetadata.RawDataLen = len(content)

	datum := &statefulpb.Datum{
		Data: &statefulpb.Datum_Logs{
			Logs: &statefulpb.Log{
				Timestamp: 12345,
				Content: &statefulpb.Log_Raw{
					Raw: content,
				},
			},
		},
	}

	return &message.StatefulMessage{
		Metadata: &msg.MessageMetadata,
		Datum:    datum,
	}
}

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(
		input,
		output,
		flushChan,
		100*time.Millisecond,
		2, // maxBatchSize
		1000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message1 := createTestStatefulMessage("a")
	input <- message1

	message2 := createTestStatefulMessage("b")
	input <- message2

	// Expect payload to be sent because buffer is full
	payload := <-output
	assert.Equal(t, 2, len(payload.MessageMetas))
	assert.Equal(t, message1.Metadata, payload.MessageMetas[0])
	assert.Equal(t, message2.Metadata, payload.MessageMetas[1])
	assert.Equal(t, "identity", payload.Encoding)
	assert.Equal(t, 2, payload.UnencodedSize)

	// Verify the payload contains valid DatumSequence
	var datumSeq statefulpb.DatumSequence
	err := proto.Unmarshal(payload.Encoded, &datumSeq)
	require.NoError(t, err)
	assert.Equal(t, 2, len(datumSeq.Data))
	assert.Equal(t, "a", datumSeq.Data[0].GetLogs().GetRaw())
	assert.Equal(t, "b", datumSeq.Data[1].GetLogs().GetRaw())

	s.Stop()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategySendsPayloadWhenBufferIsOutdated(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})
	timerInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(
		input,
		output,
		flushChan,
		timerInterval,
		100, // maxBatchSize
		1000,
		"test",
		clk,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	for round := 0; round < 3; round++ {
		m := createTestStatefulMessage("test")
		input <- m

		// It should flush in this time
		clk.Add(2 * timerInterval)

		payload := <-output
		assert.EqualValues(t, m.Metadata, payload.MessageMetas[0])

		// Verify payload contains valid DatumSequence
		var datumSeq statefulpb.DatumSequence
		err := proto.Unmarshal(payload.Encoded, &datumSeq)
		require.NoError(t, err)
		assert.Equal(t, 1, len(datumSeq.Data))
	}

	s.Stop()
	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategySendsPayloadWhenClosingInput(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(
		input,
		output,
		flushChan,
		100*time.Millisecond,
		2,
		1000,
		"test",
		clk,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message := createTestStatefulMessage("test")
	input <- message

	go func() {
		s.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	// Expect payload to be sent before timer, so we never advance the clock; if this
	// doesn't work, the test will hang
	payload := <-output
	assert.Equal(t, message.Metadata, payload.MessageMetas[0])
}

func TestBatchStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(
		input,
		output,
		flushChan,
		100*time.Millisecond,
		2,
		1000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message := createTestStatefulMessage("test")
	input <- message

	go func() {
		s.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	payload := <-output
	assert.Equal(t, message.Metadata, payload.MessageMetas[0])
}

func TestBatchStrategySynchronousFlush(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// Batch size is large so it will not flush until we trigger it manually
	// Flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// All of these messages will get buffered
	messages := []*message.StatefulMessage{
		createTestStatefulMessage("a"),
		createTestStatefulMessage("b"),
		createTestStatefulMessage("c"),
	}

	messageMeta := make([]*message.MessageMetadata, len(messages))
	for idx, m := range messages {
		input <- m
		messageMeta[idx] = m.Metadata
	}

	// Since the batch size is large there should be nothing on the output yet
	select {
	case <-output:
		assert.Fail(t, "there should be nothing on the output channel yet")
	default:
	}

	go func() {
		// Stop triggers the flush and make sure we can read the messages out now
		strategy.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	payload := <-output
	assert.ElementsMatch(t, messageMeta, payload.MessageMetas)

	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}
}

func TestBatchStrategyFlushChannel(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// Batch size is large so it will not flush until we trigger it manually
	// Flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// All of these messages will get buffered
	messages := []*message.StatefulMessage{
		createTestStatefulMessage("a"),
		createTestStatefulMessage("b"),
		createTestStatefulMessage("c"),
	}
	messageMeta := make([]*message.MessageMetadata, len(messages))
	for idx, m := range messages {
		input <- m
		messageMeta[idx] = m.Metadata
	}

	// Since the batch size is large there should be nothing on the output yet
	select {
	case <-output:
		assert.Fail(t, "there should be nothing on the output channel yet")
	default:
	}

	// Trigger a manual flush
	flushChan <- struct{}{}

	payload := <-output
	assert.ElementsMatch(t, messageMeta, payload.MessageMetas)

	// Ensure we read all of the messages
	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}

	// End the test strategy
	go func() {
		// Stop triggers the flush and make sure we can read the messages out now
		strategy.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategyMessageTooLarge(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload, 10) // Buffered to prevent deadlock
	flushChan := make(chan struct{})

	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10, // Small content size limit
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// Send a message that fits
	normalMessage := createTestStatefulMessage("small")
	input <- normalMessage

	// Send a message that's too large (will be dropped)
	largeMessage := createTestStatefulMessage("this message is way too large for the content size limit")
	input <- largeMessage

	// Trigger flush
	flushChan <- struct{}{}

	// Should only receive the normal message
	payload := <-output
	assert.Equal(t, 1, len(payload.MessageMetas))
	assert.Equal(t, normalMessage.Metadata, payload.MessageMetas[0])

	// Verify no more payloads
	select {
	case <-output:
		assert.Fail(t, "should not receive more payloads")
	default:
	}

	strategy.Stop()
}

func TestBatchStrategyInvalidDatum(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload, 10) // Buffered to prevent deadlock
	flushChan := make(chan struct{})

	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		1000,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// Send message with nil Datum
	msg1 := message.NewMessage([]byte("test"), nil, "", 0)
	invalidMsg1 := &message.StatefulMessage{
		Metadata: &msg1.MessageMetadata,
		Datum:    nil,
	}
	input <- invalidMsg1

	// Send message with wrong Datum type
	msg2 := message.NewMessage([]byte("test"), nil, "", 0)
	invalidMsg2 := &message.StatefulMessage{
		Metadata: &msg2.MessageMetadata,
		Datum:    "wrong type",
	}
	input <- invalidMsg2

	// Send a valid message
	validMsg := createTestStatefulMessage("valid")
	input <- validMsg

	// Trigger flush
	flushChan <- struct{}{}

	// Should only receive the valid message
	payload := <-output
	assert.Equal(t, 1, len(payload.MessageMetas))
	assert.Equal(t, validMsg.Metadata, payload.MessageMetas[0])

	strategy.Stop()
}

func TestBatchStrategyCompression(t *testing.T) {
	input := make(chan *message.StatefulMessage)
	output := make(chan *message.Payload, 10) // Buffered to prevent deadlock
	flushChan := make(chan struct{})

	// Use identity (no-op) compression for simplicity
	// Testing actual compression behavior is covered by the compression package tests
	compressor := compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1)

	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		time.Hour,
		100,
		10000,
		"test",
		compressor,
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// Send several messages
	for i := 0; i < 5; i++ {
		msg := createTestStatefulMessage("test message")
		input <- msg
	}

	// Trigger flush
	flushChan <- struct{}{}

	payload := <-output
	assert.Equal(t, 5, len(payload.MessageMetas))
	assert.Equal(t, "identity", payload.Encoding)
	assert.NotEmpty(t, payload.Encoded)

	// Verify the payload contains valid DatumSequence (identity compression = no compression)
	var datumSeq statefulpb.DatumSequence
	err := proto.Unmarshal(payload.Encoded, &datumSeq)
	require.NoError(t, err)
	assert.Equal(t, 5, len(datumSeq.Data))
	for _, datum := range datumSeq.Data {
		assert.Equal(t, "test message", datum.GetLogs().GetRaw())
	}

	strategy.Stop()
}
