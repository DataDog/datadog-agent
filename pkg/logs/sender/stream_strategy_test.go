// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestStreamStrategy(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	StreamStrategy.Start(input, output)

	content := []byte("a")
	message1 := message.NewMessage(content, nil, "", 0)
	input <- message1

	payload := <-output
	assert.Equal(t, message1, payload.Messages[0])
	assert.Equal(t, content, payload.Encoded)

	content = []byte("b")
	message2 := message.NewMessage(content, nil, "", 0)
	input <- message2

	payload = <-output
	assert.Equal(t, message2, payload.Messages[0])
	assert.Equal(t, content, payload.Encoded)
}

func TestStreamStrategyShouldNotBlockWhenForceStopping(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	message := message.NewMessage([]byte{}, nil, "", 0)
	go func() {
		input <- message
		close(input)
	}()

	StreamStrategy.Start(input, output)
}

func TestStreamStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	message := message.NewMessage([]byte{}, nil, "", 0)
	go func() {
		input <- message
		close(input)
		assert.Equal(t, message, <-output)
	}()

	StreamStrategy.Start(input, output)
}
