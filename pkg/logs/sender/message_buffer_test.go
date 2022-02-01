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

func TestMessageBufferLength(t *testing.T) {
	buffer := NewMessageBuffer(2, 3, 10)

	// expect buffer to be empty
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())

	// expect add to succeed
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("a"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 1)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[0].Content, []byte("a"))

	// expect add to succeed and buffer to be full
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("b"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.True(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[1].Content, []byte("b"))

	// expect add to fail because of buffer full
	assert.False(t, buffer.AddMessage(message.NewMessage([]byte("c"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.True(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[1].Content, []byte("b"))

	// expect buffer to be empty
	buffer.Clear()
	assert.Len(t, buffer.GetMessages(), 0)
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
}

func TestMessageBufferContentSize(t *testing.T) {
	buffer := NewMessageBuffer(3, 2, 10)

	// expect buffer to be empty
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())

	// expect add to succeed
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("a"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 1)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[0].Content, []byte("a"))

	// expect add to fail because message is too big
	assert.False(t, buffer.AddMessage(message.NewMessage([]byte("ccc"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 1)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())

	// expect buffer to be empty
	buffer.Clear()
	assert.Len(t, buffer.GetMessages(), 0)
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
}

func TestMessageMaxBufferSize(t *testing.T) {
	buffer := NewMessageBuffer(10, 2, 5)

	// expect buffer to be empty
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())

	// expect add to succeed
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("aa"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 1)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[0].Content, []byte("aa"))

	// expect add to succeed
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("bb"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[1].Content, []byte("bb"))

	// expect add to fail because buffer size limit has been reached (even though there is room for more messages)
	assert.False(t, buffer.AddMessage(message.NewMessage([]byte("cc"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 2)
	assert.False(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())

	// There is still room for 1 more byte and a total of 10 messages (of which we are using 2)
	assert.True(t, buffer.AddMessage(message.NewMessage([]byte("c"), nil, "", 0)))
	assert.Len(t, buffer.GetMessages(), 3)
	assert.False(t, buffer.IsEmpty())
	assert.True(t, buffer.IsFull())
	assert.Equal(t, buffer.GetMessages()[2].Content, []byte("c"))

	// expect buffer to be empty
	buffer.Clear()
	assert.Len(t, buffer.GetMessages(), 0)
	assert.True(t, buffer.IsEmpty())
	assert.False(t, buffer.IsFull())
}
