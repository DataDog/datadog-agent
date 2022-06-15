// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/assert"
)

func TestManualBatchStrategySendsPayloadWhenManualFlush(t *testing.T) {
	input := make(chan *message.Message)
	output := make(chan *message.Payload)

	s := NewManualBatchStrategy(input, output, LineSerializer, 10, 10, "test", &identityContentType{})
	s.Start()

	message1 := message.NewMessage([]byte("a"), nil, "", 0)
	input <- message1

	message2 := message.NewMessage([]byte("b"), nil, "", 0)
	input <- message2

	expectedPayload := &message.Payload{
		Messages:      []*message.Message{message1, message2},
		Encoded:       []byte("a\nb"),
		Encoding:      "identity",
		UnencodedSize: 3,
	}

	s.Flush()

	// expect payload to be sent because buffer is not full, but flush has been called
	assert.Equal(t, expectedPayload, <-output)
	s.Stop()

	if _, isOpen := <-input; isOpen {
		assert.Fail(t, "input should be closed")
	}
}
