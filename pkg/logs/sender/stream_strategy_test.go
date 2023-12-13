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

	s := NewStreamStrategy(input, output, IdentityContentType)
	s.Start()

	content := []byte("a")
	message1 := message.NewMessage(content, nil, "", 0)
	input <- message1

	payload := <-output
	assert.Equal(t, message1, payload.Messages[0])
	assert.Equal(t, 1, payload.UnencodedSize)
	assert.Equal(t, content, payload.Encoded)

	content = []byte("b")
	message2 := message.NewMessage(content, nil, "", 0)
	input <- message2

	payload = <-output
	assert.Equal(t, message2, payload.Messages[0])
	assert.Equal(t, 1, payload.UnencodedSize)
	assert.Equal(t, content, payload.Encoded)
	s.Stop()
}

func TestStreamStrategyShouldNotBlockWhenForceStopping(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	s := NewStreamStrategy(input, output, IdentityContentType)

	message := message.NewMessage([]byte{}, nil, "", 0)
	go func() {
		input <- message
		s.Stop()
	}()

	s.Start()
}

func TestStreamStrategyShouldNotBlockWhenStoppingGracefully(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	s := NewStreamStrategy(input, output, IdentityContentType)

	message := message.NewMessage([]byte{}, nil, "", 0)
	go func() {
		input <- message
		s.Stop()
		assert.Equal(t, message, <-output)
	}()

	s.Start()
}
