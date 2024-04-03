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

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan message.TimedMessage[*message.Message])
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(input, output, flushChan, LineSerializer, 100*time.Millisecond, 2, 2, "test", &identityContentType{})
	s.Start()

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message.NewTimedMessage(message1)

	message2 := message.NewMessage([]byte("b"), nil, "", 0)
	input <- message.NewTimedMessage(message2)

	expectedPayload := &message.Payload{
		Messages:      []*message.Message{message1, message2},
		Encoded:       []byte("a\nb"),
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
	input := make(chan message.TimedMessage[*message.Message])
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})
	timerInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(input, output, flushChan, LineSerializer, timerInterval, 100, 100, "test", clk, &identityContentType{})
	s.Start()

	for round := 0; round < 3; round++ {
		m := message.NewMessage([]byte("a"), nil, "", 0)
		input <- message.NewTimedMessage(m)

		// it should flush in this time
		clk.Add(2 * timerInterval)

		payload := <-output
		assert.EqualValues(t, m, payload.Messages[0])
	}
	s.Stop()
	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}

func TestBatchStrategySendsPayloadWhenClosingInput(t *testing.T) {
	input := make(chan message.TimedMessage[*message.Message])
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	clk := clock.NewMock()
	s := newBatchStrategyWithClock(input, output, flushChan, LineSerializer, 100*time.Millisecond, 2, 2, "test", clk, &identityContentType{})
	s.Start()

	msg := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message.NewTimedMessage(msg)

	go func() {
		s.Stop()
	}()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	// expect payload to be sent before timer, so we never advance the clock; if this
	// doesn't work, the test will hang
	payload := <-output
	assert.Equal(t, msg, payload.Messages[0])
}

func TestBatchStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan message.TimedMessage[*message.Message])
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	s := NewBatchStrategy(input, output, flushChan, LineSerializer, 100*time.Millisecond, 2, 2, "test", &identityContentType{})
	s.Start()
	msg := message.NewMessage([]byte{}, nil, "", 0)

	input <- message.NewTimedMessage(msg)

	go func() {
		s.Stop()
	}()
	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}

	assert.Equal(t, msg, (<-output).Messages[0])
}

func TestBatchStrategySynchronousFlush(t *testing.T) {
	input := make(chan message.TimedMessage[*message.Message])
	// output needs to be buffered so the flush has somewhere to write to without blocking
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// batch size is large so it will not flush until we trigger it manually
	// flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(input, output, flushChan, LineSerializer, time.Hour, 100, 100, "test", &identityContentType{})
	strategy.Start()

	// all of these messages will get buffered
	messages := []*message.Message{
		message.NewMessage([]byte("a"), nil, "", 0),
		message.NewMessage([]byte("b"), nil, "", 0),
		message.NewMessage([]byte("c"), nil, "", 0),
	}
	for _, m := range messages {
		input <- message.NewTimedMessage(m)
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

	assert.ElementsMatch(t, messages, (<-output).Messages)

	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}
}

func TestBatchStrategyFlushChannel(t *testing.T) {
	input := make(chan message.TimedMessage[*message.Message])
	output := make(chan *message.Payload)
	flushChan := make(chan struct{})

	// batch size is large so it will not flush until we trigger it manually
	// flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(input, output, flushChan, LineSerializer, time.Hour, 100, 100, "test", &identityContentType{})
	strategy.Start()

	// all of these messages will get buffered
	messages := []*message.Message{
		message.NewMessage([]byte("a"), nil, "", 0),
		message.NewMessage([]byte("b"), nil, "", 0),
		message.NewMessage([]byte("c"), nil, "", 0),
	}
	for _, m := range messages {
		input <- message.NewTimedMessage(m)
	}

	// since the batch size is large there should be nothing on the output yet
	select {
	case <-output:
		assert.Fail(t, "there should be nothing on the output channel yet")
	default:
	}

	// Trigger a manual flush
	flushChan <- struct{}{}

	assert.ElementsMatch(t, messages, (<-output).Messages)

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
