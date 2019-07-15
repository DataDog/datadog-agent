// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestStreamStrategy(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		assert.Equal(t, content, payload)
		return nil
	}

	go StreamStrategy.Send(input, output, success)

	content = []byte("a")
	message1 := message.NewMessage(content, nil, "")
	input <- message1

	assert.Equal(t, message1, <-output)

	content = []byte("b")
	message2 := message.NewMessage(content, nil, "")
	input <- message2

	assert.Equal(t, message2, <-output)
}

func TestStreamStrategyShouldNotBlockWhenForceStopping(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		return context.Canceled
	}

	message := message.NewMessage(content, nil, "")
	go func() {
		input <- message
		close(input)
	}()

	StreamStrategy.Send(input, output, success)
}

func TestStreamStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Message)

	var content []byte
	success := func(payload []byte) error {
		return nil
	}

	message := message.NewMessage(content, nil, "")
	go func() {
		input <- message
		close(input)
		assert.Equal(t, message, <-output)
	}()

	StreamStrategy.Send(input, output, success)
}
