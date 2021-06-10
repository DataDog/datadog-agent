// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		assert.Equal(t, content, payload)
		return nil
	}

	done := make(chan bool)
	go func() {
		NewBatchStrategy(LineSerializer, 100*time.Millisecond, 0, 2, 2, "test").Send(input, output, success)
		close(done)
	}()

	content = []byte("a\nb")

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message1

	message2 := message.NewMessage([]byte("b"), nil, "", 0)
	input <- message2

	// expect payload to be sent because buffer is full
	assert.Equal(t, message1, <-output)
	assert.Equal(t, message2, <-output)
	close(input)
	<-done
}

func TestBatchStrategySendsPayloadWhenBufferIsOutdated(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message, 10)
	timerInterval := 100 * time.Millisecond

	// payload sends are blocked until we've confirmed that the we buffer the correct number of pending payloads
	send := func(payload []byte) error {
		return nil
	}

	strategy := NewBatchStrategy(LineSerializer, timerInterval, 0, 100, 100, "test")
	done := make(chan bool)
	go func() {
		strategy.Send(input, output, send)
		close(done)
	}()

	for round := 0; round < 3; round++ {
		m := message.NewMessage([]byte("a"), nil, "", 0)
		input <- m

		// it should have flushed in this time
		<-time.After(2 * timerInterval)

		select {
		case mOut := <-output:
			assert.EqualValues(t, m, mOut)
		default:
			assert.Fail(t, "the output channel should not be empty")
		}
	}

	close(input)
	<-done
}

func TestBatchStrategySendsPayloadWhenClosingInput(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		assert.Equal(t, content, payload)
		return nil
	}

	done := make(chan bool)
	go func() {
		NewBatchStrategy(LineSerializer, 100*time.Millisecond, 0, 2, 2, "test").Send(input, output, success)
		close(done)
	}()

	content = []byte("a")

	message := message.NewMessage(content, nil, "", 0)
	input <- message

	start := time.Now()
	close(input)

	// expect payload to be sent before timer
	assert.Equal(t, message, <-output)
	end := start.Add(100 * time.Millisecond)
	now := time.Now()
	assert.True(t, now.Before(end) || now.Equal(end))
	<-done
}

func TestBatchStrategyShouldNotBlockWhenForceStopping(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		return context.Canceled
	}

	message := message.NewMessage(content, nil, "", 0)
	go func() {
		input <- message
		close(input)
	}()

	NewBatchStrategy(LineSerializer, 100*time.Millisecond, 0, 2, 2, "test").Send(input, output, success)
}

func TestBatchStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		return nil
	}

	message := message.NewMessage(content, nil, "", 0)
	go func() {
		input <- message
		close(input)
		assert.Equal(t, message, <-output)
	}()

	NewBatchStrategy(LineSerializer, 100*time.Millisecond, 0, 2, 2, "test").Send(input, output, success)
}

func TestBatchStrategyConcurrentSends(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message, 10)
	waitChan := make(chan bool)

	// payload sends are blocked until we've confirmed that the we buffer the correct number of pending payloads
	stuckSend := func(payload []byte) error {
		<-waitChan
		return nil
	}

	strategy := NewBatchStrategy(LineSerializer, 100*time.Millisecond, 2, 1, 100, "test")
	done := make(chan bool)
	go func() {
		strategy.Send(input, output, stuckSend)
		close(done)
	}()

	messages := []*message.Message{
		// the first two messages will be blocked in concurrent send goroutines
		message.NewMessage([]byte("a"), nil, "", 0),
		message.NewMessage([]byte("b"), nil, "", 0),
		// the third message will be read out by the main batch sender loop and will be blocked waiting for one of the
		// first two concurrent sends to complete
		message.NewMessage([]byte("c"), nil, "", 0),
	}

	for _, m := range messages {
		input <- m
	}

	select {
	case input <- message.NewMessage([]byte("c"), nil, "", 0):
		assert.Fail(t, "should not have been able to write into the channel as the input channel is expected to be backed up due to reaching max concurrent sends")
	default:
	}

	close(waitChan)
	close(input)
	<-done
	close(output)

	var receivedMessages []*message.Message
	for m := range output {
		receivedMessages = append(receivedMessages, m)
	}

	// order in which messages are received here is not deterministic so compare values
	assert.ElementsMatch(t, messages, receivedMessages)
}

func TestBatchStrategySynchronousFlush(t *testing.T) {
	input := make(chan *message.Message)
	// output needs to be buffered so the flush has somewhere to write to without blocking
	output := make(chan *message.Message, 3)
	send := func(payload []byte) error {
		return nil
	}

	// batch size is large so it will not flush until we trigger it manually
	// flush time is large so it won't automatically trigger during this test
	strategy := NewBatchStrategy(LineSerializer, time.Hour, 0, 100, 100, "test")
	done := make(chan bool)
	go func() {
		strategy.Send(input, output, send)
		close(done)
	}()

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

	var receivedMessages []*message.Message
	for range messages {
		receivedMessages = append(receivedMessages, <-output)
	}
	assert.ElementsMatch(t, messages, receivedMessages)

	close(input)
	<-done

	select {
	case <-output:
		assert.Fail(t, "the output channel should still be empty")
	default:
	}
}
