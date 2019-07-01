// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// newBatchStrategyWithLimits returns a new batchStrategy.
func newBatchStrategyWithLimits(serializer Serializer, batchSize int, contentSize int, timeout time.Duration) Strategy {
	return &batchStrategy{
		buffer:       NewMessageBuffer(batchSize, contentSize),
		serializer:   serializer,
		batchTimeout: timeout,
	}
}

func TestBatchStrategySendsPayloadWhenBufferIsFull(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		assert.Equal(t, content, payload)
		return nil
	}

	go newBatchStrategyWithLimits(NewLineSerializer(), 2, 2, 100*time.Millisecond).Send(input, output, success)

	content = []byte("a\nb")

	message1 := message.NewMessage([]byte("a"), nil, "")
	input <- message1

	message2 := message.NewMessage([]byte("b"), nil, "")
	input <- message2

	// expect payload to be sent because buffer is full
	assert.Equal(t, message1, <-output)
	assert.Equal(t, message2, <-output)
}

func TestBatchStrategySendsPayloadWhenBufferIsOutdated(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		assert.Equal(t, content, payload)
		return nil
	}

	go newBatchStrategyWithLimits(NewLineSerializer(), 2, 2, 100*time.Millisecond).Send(input, output, success)

	content = []byte("a")

	message1 := message.NewMessage([]byte(content), nil, "")
	input <- message1

	// expect payload to be sent after timer
	start := time.Now()
	assert.Equal(t, message1, <-output)
	assert.True(t, time.Now().After(start.Add(100*time.Millisecond)))

	content = []byte("b\nc")

	message2 := message.NewMessage([]byte("b"), nil, "")
	input <- message2

	message3 := message.NewMessage([]byte("c"), nil, "")
	input <- message3

	// expect payload to be sent because buffer is full
	assert.Equal(t, message2, <-output)
	assert.Equal(t, message3, <-output)
}

func TestBatchStrategySendsPayloadWhenClosingInput(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		assert.Equal(t, content, payload)
		return nil
	}

	go newBatchStrategyWithLimits(NewLineSerializer(), 2, 2, 100*time.Millisecond).Send(input, output, success)

	content = []byte("a")

	message := message.NewMessage([]byte(content), nil, "")
	input <- message

	start := time.Now()
	close(input)

	// expect payload to be sent before timer
	assert.Equal(t, message, <-output)
	assert.True(t, time.Now().Before(start.Add(100*time.Millisecond)))
}

func TestBatchStrategyShouldNotBlockWhenForceStopping(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		return context.Canceled
	}

	message := message.NewMessage([]byte(content), nil, "")
	go func() {
		input <- message
		close(input)
	}()

	newBatchStrategyWithLimits(NewLineSerializer(), 2, 2, 100*time.Millisecond).Send(input, output, success)
}

func TestBatchStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		return nil
	}

	message := message.NewMessage([]byte(content), nil, "")
	go func() {
		input <- message
		close(input)
		assert.Equal(t, message, <-output)
	}()

	newBatchStrategyWithLimits(NewLineSerializer(), 2, 2, 100*time.Millisecond).Send(input, output, success)
}
