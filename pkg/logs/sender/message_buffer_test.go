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

func TestMessageBufferSize(t *testing.T) {
	buffer := NewMessageBuffer(2, 3)

	// expect buffer to be empty
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())

	// expect add to success
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("a"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 1)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[0].GetContent(), []byte("a"))

	// expect add to success and buffer to be full
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("b"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.True(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[1].GetContent(), []byte("b"))

	// expect add to success to fail because of buffer full
	assert.False(t, buffer.AddMessage(message.NewMessage([]byte("c"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.True(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[1].GetContent(), []byte("b"))

	// expect buffer to be empty
	buffer.Clear()
	assert.Len(t, buffer.GetMessages(), 0)
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
}

func TestMessageBufferContentSize(t *testing.T) {
	buffer := NewMessageBuffer(3, 2)

	// expect buffer to be empty
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())

	// expect add to success
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("a"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 1)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[0].GetContent(), []byte("a"))

	// expect add to success and buffer to be full
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("b"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.True(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[1].GetContent(), []byte("b"))

	// expect add to success to fail because of buffer full
	assert.False(t, buffer.AddMessage(message.NewMessage([]byte("c"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.True(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[1].GetContent(), []byte("b"))

	// expect buffer to be empty
	buffer.Clear()
	assert.Len(t, buffer.GetMessages(), 0)
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
}
