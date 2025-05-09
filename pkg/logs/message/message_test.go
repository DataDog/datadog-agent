// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessage(t *testing.T) {

	message := NewMessage([]byte("hello"), nil, "", 0)
	assert.Equal(t, "hello", string(message.GetContent()))

	message.SetContent([]byte("world"))
	assert.Equal(t, "world", string(message.GetContent()))
	assert.Equal(t, StatusInfo, message.GetStatus())
}

func TestNewPayload(t *testing.T) {
	messages := []*Message{
		NewMessage([]byte("hello"), nil, "", 0),
		NewMessage([]byte("world"), nil, "", 0),
		NewMessage([]byte("test"), nil, "", 0),
	}
	messageMetas := make([]*MessageMetadata, len(messages))
	for i, msg := range messages {
		messageMetas[i] = &msg.MessageMetadata
	}
	encoded := []byte("encoded content")
	encoding := "gzip"
	unencodedSize := 100

	payload := NewPayload(messageMetas, encoded, encoding, unencodedSize)

	// Test basic payload properties
	assert.Equal(t, 3, len(payload.MessageMetas))
	assert.Equal(t, encoded, payload.Encoded)
	assert.Equal(t, encoding, payload.Encoding)
	assert.Equal(t, unencodedSize, payload.UnencodedSize)

	// Test Count method
	assert.Equal(t, int64(3), payload.Count())

	// Test Size method (each message is 5, 5, and 4 bytes respectively)
	assert.Equal(t, int64(14), payload.Size())
}
func TestPayloadPreservesMessageOrder(t *testing.T) {
	messages := []*Message{
		NewMessage([]byte("1"), nil, "", 1),    // datalen = 1
		NewMessage([]byte("22"), nil, "", 2),   // datalen = 2
		NewMessage([]byte("333"), nil, "", 3),  // datalen = 3
		NewMessage([]byte("4444"), nil, "", 4), // datalen = 4
	}
	messageMetas := make([]*MessageMetadata, len(messages))
	for i, msg := range messages {
		messageMetas[i] = &msg.MessageMetadata
	}

	payload := NewPayload(messageMetas, []byte(""), "", 0)

	expectedLengths := []int{1, 2, 3, 4}
	assert.Equal(t, len(expectedLengths), len(payload.MessageMetas), "Should have same number of message metas")

	for i, msg := range messages {
		assert.Equal(t, msg.RawDataLen, payload.MessageMetas[i].RawDataLen, "Message at index %d should have RawDataLen of %d", i, msg.GetContent())
		assert.Equal(t, msg.IngestionTimestamp, payload.MessageMetas[i].IngestionTimestamp, "Message at index %d should have ingestion timestamp %d", i, msg.IngestionTimestamp)
	}
}
