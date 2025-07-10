// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

type MockSerializer struct {
	arraySerializer    Serializer
	mu                 sync.Mutex
	failOnNthSerialize int
	serializeCallCount int
	failOnNthFinish    int
	finishCallCount    int
}

// NewMockSerializer creates a new MockSerializer
func NewMockSerializer(failOnNthSerialize int, failOnNthFinish int) *MockSerializer {
	return &MockSerializer{
		arraySerializer:    NewArraySerializer(),
		failOnNthSerialize: failOnNthSerialize,
		failOnNthFinish:    failOnNthFinish,
		serializeCallCount: 0,
		finishCallCount:    0,
	}
}

// Serialize transforms all messages into a array string allowing failures.
func (s *MockSerializer) Serialize(message *message.Message, writer io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.serializeCallCount++
	if s.failOnNthSerialize > 0 && s.serializeCallCount == s.failOnNthSerialize {
		return errors.New("mock Nth Serialize failure")
	}

	return s.arraySerializer.Serialize(message, writer)
}

// Finish writes the closing bracket for JSON array allowing failures.
func (s *MockSerializer) Finish(writer io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.finishCallCount++

	if s.failOnNthFinish > 0 && s.finishCallCount == s.failOnNthFinish {
		return errors.New("mock Nth Finish failure")
	}

	return s.arraySerializer.Finish(writer)
}

// Reset resets the serializer to its initial state
func (s *MockSerializer) Reset() {
	s.arraySerializer.Reset()
}

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(input, output, flushChan, NewMockServerlessMeta(false), NewArraySerializer(), 100*time.Millisecond, 2, 2, "test", compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
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

func TestBatchStrategyOverflowsOnTooLargeMessage(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(input, output, flushChan, NewMockServerlessMeta(false), NewArraySerializer(), 100*time.Millisecond, 2, 2, "test", compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
	s.Start()

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message1

	// message2 will overflow the first payload causing message1 to flush, but then also fail to be added to the buffer
	// because it's too big on it's own. message2 is dropped.
	message2 := message.NewMessage([]byte("bbbbbb"), nil, "", 0)
	input <- message2

	expectedPayload := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&message1.MessageMetadata},
		Encoded:       []byte(`[a]`),
		Encoding:      "identity",
		UnencodedSize: 3,
	}

	// expect payload to be sent because buffer is full
	assert.Equal(t, expectedPayload, <-output)
	s.Stop()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategySendsPayloadWhenBufferIsOutdated(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})
	timerInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(input, output, flushChan, NewMockServerlessMeta(false), NewArraySerializer(), timerInterval, 100, 100, "test", clk, compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
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
	s := newBatchStrategyWithClock(input, output, flushChan, NewMockServerlessMeta(false), NewArraySerializer(), 100*time.Millisecond, 2, 2, "test", clk, compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
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

	s := NewBatchStrategy(input, output, flushChan, NewMockServerlessMeta(false), NewArraySerializer(), 100*time.Millisecond, 2, 2, "test", compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
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
	strategy := NewBatchStrategy(input, output, flushChan, NewMockServerlessMeta(false), NewArraySerializer(), time.Hour, 100, 100, "test", compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
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
	strategy := NewBatchStrategy(input, output, flushChan, NewMockServerlessMeta(false), NewArraySerializer(), time.Hour, 100, 100, "test", compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
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

func TestBatchStrategyDiscardsPayloadWhenSerializerFails(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload, 1) // Ensure output is buffered if test sends multiple payloads before asserting
	flushChan := make(chan struct{})
	// Fail on the 2nd call to Serialize, never fail Finish.
	serializer := NewMockSerializer(2, 0)
	s := NewBatchStrategy(input, output, flushChan, NewMockServerlessMeta(false), serializer, 100*time.Millisecond, 2, 2, "test", compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
	s.Start()

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message1 // 1st Serialize call, should succeed.

	// This message's serialization is intended to fail.
	message2 := message.NewMessage([]byte("b"), nil, "", 0)
	input <- message2 // 2nd Serialize call, should fail.

	message3 := message.NewMessage([]byte("c"), nil, "", 0)
	input <- message3

	message4 := message.NewMessage([]byte("d"), nil, "", 0)
	input <- message4

	expectedPayload := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&message3.MessageMetadata, &message4.MessageMetadata},
		Encoded:       []byte(`[c,d]`),
		Encoding:      "identity",
		UnencodedSize: 5,
	}

	actualPayload := <-output
	assert.Equal(t, expectedPayload, actualPayload)

	s.Stop()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategyDiscardsPayloadWhenSerializerFailsOnFinish(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload, 1) // Ensure output is buffered if test sends multiple payloads before asserting
	flushChan := make(chan struct{})
	// Fail on the 1st call to Finish, never fail Serialize.
	serializer := NewMockSerializer(0, 1)
	s := NewBatchStrategy(input, output, flushChan, NewMockServerlessMeta(false), serializer, 100*time.Millisecond, 2, 2, "test", compressionfx.NewMockCompressor().NewCompressor(compression.NoneKind, 1), metrics.NewNoopPipelineMonitor(""), "test")
	s.Start()

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message1 // 1st Serialize call, should succeed.

	message2 := message.NewMessage([]byte("b"), nil, "", 0)
	input <- message2 // 2nd Serialize call, should fail.
	// 1st Finish call, should fail.

	message3 := message.NewMessage([]byte("c"), nil, "", 0)
	input <- message3

	message4 := message.NewMessage([]byte("d"), nil, "", 0)
	input <- message4

	expectedPayload := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&message3.MessageMetadata, &message4.MessageMetadata},
		Encoded:       []byte(`[c,d]`),
		Encoding:      "identity",
		UnencodedSize: 5,
	}

	actualPayload := <-output
	assert.Equal(t, expectedPayload, actualPayload)

	s.Stop()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}
