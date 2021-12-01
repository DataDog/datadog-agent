// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	NewBatchStrategy(LineSerializer, 100*time.Millisecond, 2, 2, "test", &identityContentType{}).Start(input, output)

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message1

	message2 := message.NewMessage([]byte("b"), nil, "", 0)
	input <- message2

	expectedPayload := &message.Payload{
		Messages: []*message.Message{message1, message2},
		Encoded:  []byte("a\nb"),
	}

	// expect payload to be sent because buffer is full
	assert.Equal(t, expectedPayload, <-output)
	close(input)
}

func TestBatchStrategySendsPayloadWhenBufferIsOutdated(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)
	timerInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	newBatchStrategyWithClock(LineSerializer, timerInterval, 100, 100, "test", clk, &identityContentType{}).Start(input, output)

	for round := 0; round < 3; round++ {
		m := message.NewMessage([]byte("a"), nil, "", 0)
		input <- m

		// it should have flushed in this time
		clk.Add(2 * timerInterval)

		select {
		case payload := <-output:
			assert.EqualValues(t, m, payload.Messages[0])
		default:
			assert.Fail(t, "the output channel should not be empty")
		}
	}

	close(input)
}

func TestBatchStrategySendsPayloadWhenClosingInput(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	clk := clock.NewMock()
	newBatchStrategyWithClock(LineSerializer, 100*time.Millisecond, 2, 2, "test", clk, &identityContentType{}).Start(input, output)

	message := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message

	close(input)

	// expect payload to be sent before timer, so we never advance the clock; if this
	// doesn't work, the test will hang
	payload := <-output
	assert.Equal(t, message, payload.Messages[0])
}

func TestBatchStrategyShouldNotBlockWhenForceStopping(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	message := message.NewMessage([]byte{}, nil, "", 0)
	go func() {
		input <- message
		close(input)
	}()

	NewBatchStrategy(LineSerializer, 100*time.Millisecond, 2, 2, "test", &identityContentType{}).Start(input, output)
}

func TestBatchStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	message := message.NewMessage([]byte{}, nil, "", 0)
	go func() {
		input <- message
		close(input)
		assert.Equal(t, message, <-output)
	}()

	NewBatchStrategy(LineSerializer, 100*time.Millisecond, 2, 2, "test", &identityContentType{}).Start(input, output)
}

func TestBatchStrategySynchronousFlush(t *testing.T) {
	input := make(chan *message.Message)
	// output needs to be buffered so the flush has somewhere to write to without blocking
	output := make(chan *message.Payload)

	// batch size is large so it will not flush until we trigger it manually
	// flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(LineSerializer, time.Hour, 100, 100, "test", &identityContentType{})
	strategy.Start(input, output)

	// all of these messages will get buffered
	messages := []*message.Message{
		message.NewMessage([]byte("a"), nil, "", 0),
		message.NewMessage([]byte("b"), nil, "", 0),
		message.NewMessage([]byte("c"), nil, "", 0),
	}
	for _, m := range messages {
		input <- m
	}

	// since the batch size is large there should be nothing on the output yet
	select {
	case <-output:
		assert.Fail(t, "there should be nothing on the output channel yet")
	default:
	}

	// trigger the flush and make sure we can read the messages out now
	strategy.Flush(context.Background())

	assert.ElementsMatch(t, messages, (<-output).Messages)

	close(input)

	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}
}
