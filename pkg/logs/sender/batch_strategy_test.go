// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(
		input,
		output,
		flushChan,
		NewMockServerlessMeta(false),
		100*time.Millisecond,
		2,
		2,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message1

	message2 := message.NewMessage([]byte("b"), nil, "", 0)
	input <- message2

	expectedPayload := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&message1.MessageMetadata, &message2.MessageMetadata},
		Encoded:       []byte(`[a,b]`),
		Encoding:      "identity",
		UnencodedSize: 5,
	}

	// expect payload to be sent because buffer is full
	assert.Equal(t, expectedPayload, <-output)
	s.Stop()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

// Test removed - batch overflow logic now tested in batch_test.go

func TestBatchStrategySendsPayloadWhenBufferIsOutdated(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})
	timerInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(
		input,
		output,
		flushChan,
		NewMockServerlessMeta(false),
		timerInterval,
		100,
		100,
		"test",
		clk,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	for round := 0; round < 3; round++ {
		m := message.NewMessage([]byte("a"), nil, "", 0)
		input <- m

		// it should flush in this time
		clk.Add(2 * timerInterval)

		payload := <-output
		assert.EqualValues(t, &m.MessageMetadata, payload.MessageMetas[0])
	}
	s.Stop()
	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategySendsPayloadWhenClosingInput(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(
		input,
		output,
		flushChan,
		NewMockServerlessMeta(false),
		100*time.Millisecond,
		2,
		2,
		"test",
		clk,
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()

	message := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message

	go func() {
		s.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	// expect payload to be sent before timer, so we never advance the clock; if this
	// doesn't work, the test will hang
	payload := <-output
	assert.Equal(t, &message.MessageMetadata, payload.MessageMetas[0])
}

func TestBatchStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(input,
		output,
		flushChan,
		NewMockServerlessMeta(false),
		100*time.Millisecond,
		2,
		2,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	s.Start()
	message := message.NewMessage([]byte{}, nil, "", 0)

	input <- message

	go func() {
		s.Stop()
	}()
	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	assert.Equal(t, &message.MessageMetadata, (<-output).MessageMetas[0])
}

func TestBatchStrategySynchronousFlush(t *testing.T) {
	input := make(chan *message.Message)
	// output needs to be buffered so the flush has somewhere to write to without blocking
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// batch size is large so it will not flush until we trigger it manually
	// flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(input,
		output,
		flushChan,
		NewMockServerlessMeta(false),
		time.Hour,
		100,
		100,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// all of these messages will get buffered
	messages := []*message.Message{
		message.NewMessage([]byte("a"), nil, "", 0),
		message.NewMessage([]byte("b"), nil, "", 0),
		message.NewMessage([]byte("c"), nil, "", 0),
	}

	messageMeta := make([]*message.MessageMetadata, len(messages))
	for idx, m := range messages {
		input <- m
		messageMeta[idx] = &m.MessageMetadata
	}

	// since the batch size is large there should be nothing on the output yet
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

	assert.ElementsMatch(t, messageMeta, (<-output).MessageMetas)

	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}
}

func TestBatchStrategyFlushChannel(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// batch size is large so it will not flush until we trigger it manually
	// flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		NewMockServerlessMeta(false),
		time.Hour,
		100,
		100,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()

	// all of these messages will get buffered
	messages := []*message.Message{
		message.NewMessage([]byte("a"), nil, "", 0),
		message.NewMessage([]byte("b"), nil, "", 0),
		message.NewMessage([]byte("c"), nil, "", 0),
	}
	messageMeta := make([]*message.MessageMetadata, len(messages))
	for idx, m := range messages {
		input <- m
		messageMeta[idx] = &m.MessageMetadata
	}
	// since the batch size is large there should be nothing on the output yet
	select {
	case <-output:
		assert.Fail(t, "there should be nothing on the output channel yet")
	default:
	}

	// Trigger a manual flush
	flushChan <- struct{}{}

	assert.ElementsMatch(t, messageMeta, (<-output).MessageMetas)

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

func TestBatchStrategyMRFRouting(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload, 2) // Buffer for two payloads
	flushChan := make(chan struct{})
	var batchSize = 100
	var contentSize = 100

	strategy := NewBatchStrategy(
		input,
		output,
		flushChan,
		NewMockServerlessMeta(false),
		100*time.Millisecond,
		batchSize,
		contentSize,
		"test",
		compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1),
		metrics.NewNoopPipelineMonitor(""),
		"test")
	strategy.Start()
	normalMessage := message.NewMessage([]byte("normal message"), nil, "", 0)

	mrfMessage := message.NewMessage([]byte("mrf message"), nil, "", 0)
	mrfMessage.IsMRFAllow = true

	input <- normalMessage
	input <- mrfMessage

	flushChan <- struct{}{}

	// Should receive two payloads: main and MRF
	payloads := make([]*message.Payload, 2)
	payloads[0] = <-output
	payloads[1] = <-output

	var hasNormal, hasMRF bool
	for _, payload := range payloads {
		if payload.IsMRF() {
			hasMRF = true
		} else {
			hasNormal = true
		}
	}
	assert.True(t, hasNormal, "Should have received normal payload")
	assert.True(t, hasMRF, "Should have received MRF payload")

	strategy.Stop()
}
