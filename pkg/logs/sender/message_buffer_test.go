// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestMessageBufferRequestSize(t *testing.T) {
	mb := NewMessageBuffer(2, 1000)
	source := config.NewLogSource("", &config.LogsConfig{})
	//Add a first message lower than request size, should append and not trigger a send yet
	success := mb.TryAddMessage(newMessage(make([]byte, 500), source, ""))
	assert.True(t, success)
	//Try to add a second message above the request size, should not append and trigger a send
	success = mb.TryAddMessage(newMessage(make([]byte, 501), source, ""))
	assert.False(t, success)
	mb.Clear()
	//Clearing, try to add the previous message again, should append and not trigger a send
	success = mb.TryAddMessage(newMessage(make([]byte, 501), source, ""))
	assert.True(t, success)
}

func TestMessageBufferBatchCount(t *testing.T) {
	mb := NewMessageBuffer(2, 1000)
	source := config.NewLogSource("", &config.LogsConfig{})
	//Add a first message lower than request size, should append
	success := mb.TryAddMessage(newMessage(make([]byte, 10), source, ""))
	assert.True(t, success)
	//Try to add a second message lower the request size, should append
	success = mb.TryAddMessage(newMessage(make([]byte, 10), source, ""))
	assert.True(t, success)
	//Try to add a third message should fail append
	success = mb.TryAddMessage(newMessage(make([]byte, 10), source, ""))
	assert.False(t, success)
	mb.Clear()
	//Clearing, add a new message, should append and not trigger a send
	success = mb.TryAddMessage(newMessage(make([]byte, 10), source, ""))
	assert.True(t, success)
}

func TestMessageBufferBuild(t *testing.T) {
	mb := NewMessageBuffer(2, 1000)
	source := config.NewLogSource("", &config.LogsConfig{})
	buffer := string(mb.Build())
	assert.Equal(t, "[", buffer)
	mb.Clear()
	mb.TryAddMessage(newMessage([]byte("messagebuffer"), source, ""))
	buffer = string(mb.Build())
	assert.Equal(t, "[messagebuffer]", buffer)
	mb.Clear()
	mb.TryAddMessage(newMessage([]byte("messagebuffer"), source, ""))
	mb.TryAddMessage(newMessage([]byte("messagebuffer"), source, ""))
	buffer = string(mb.Build())
	assert.Equal(t, "[messagebuffer,messagebuffer]", buffer)
}

func TestMessageBufferIsFullEmpty(t *testing.T) {
	mb := NewMessageBuffer(2, 1000)
	assert.True(t, mb.IsEmpty())
	assert.False(t, mb.IsFull())
	source := config.NewLogSource("", &config.LogsConfig{})
	mb.TryAddMessage(newMessage([]byte("messagebuffer"), source, ""))
	assert.False(t, mb.IsEmpty())
	assert.False(t, mb.IsFull())
	mb.TryAddMessage(newMessage([]byte("messagebuffer"), source, ""))
	assert.False(t, mb.IsEmpty())
	assert.True(t, mb.IsFull())
}

func TestMessageBufferGetMessages(t *testing.T) {
	mb := NewMessageBuffer(2, 1000)
	source := config.NewLogSource("", &config.LogsConfig{})
	msgs := mb.GetMessages()
	assert.Equal(t, 0, len(msgs))
	m1 := newMessage([]byte("messagebuffer"), source, "")
	mb.TryAddMessage(m1)
	msgs = mb.GetMessages()
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, m1, msgs[0])

}
