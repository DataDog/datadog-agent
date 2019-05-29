// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MessageBuffer accumulates messages and the bytes for batch sending.
type MessageBuffer struct {
	messageBuffer []*message.Message
	byteBuffer    []byte
}

// NewMessageBuffer returns a new MessageBuffer.
func NewMessageBuffer(maxBatchCount, maxRequestSize int) *MessageBuffer {
	return &MessageBuffer{
		messageBuffer: make([]*message.Message, 0, maxBatchCount),
		byteBuffer:    make([]byte, 1, maxRequestSize),
	}
}

// TryAddMessage attempts to add a new message,
// returns false if it failed.
func (mb *MessageBuffer) TryAddMessage(m *message.Message) bool {
	if len(mb.messageBuffer) < cap(mb.messageBuffer) && mb.hasSpaceInByteBuffer(m.Content) {
		mb.messageBuffer = append(mb.messageBuffer, m)
		mb.appendByteBuffer(m.Content)
		return true
	}
	return false
}

// IsEmpty returns true if the buffer is empty.
func (mb *MessageBuffer) IsEmpty() bool {
	return len(mb.messageBuffer) == 0
}

// IsFull returns true if the buffer is full.
func (mb *MessageBuffer) IsFull() bool {
	return len(mb.messageBuffer) == cap(mb.messageBuffer)
}

// Clear removes all elements from the buffer.
func (mb *MessageBuffer) Clear() {
	mb.messageBuffer = mb.messageBuffer[:0]
	mb.byteBuffer = mb.byteBuffer[:1] // keep the first byte, it's used for : '['
}

// GetPayload returns the concatanated messages in JSON encoded format.
func (mb *MessageBuffer) GetPayload() []byte {
	// here we write the json '[' and ']'
	mb.byteBuffer[0] = '['
	if len(mb.messageBuffer) > 0 {
		mb.byteBuffer[len(mb.byteBuffer)-1] = ']'
	}
	return mb.byteBuffer
}

// GetMessages returns the buffered messages.
func (mb *MessageBuffer) GetMessages() []*message.Message {
	return mb.messageBuffer
}

// hasSpaceInByteBuffer returns if there is still some room in the buffer
// for the content.
func (mb *MessageBuffer) hasSpaceInByteBuffer(content []byte) bool {
	return len(mb.byteBuffer)+len(content)+1 < cap(mb.byteBuffer)
}

// appendByteBuffer appends the content to the buffer.
func (mb *MessageBuffer) appendByteBuffer(content []byte) {
	// increase the slice length, TODO can optimized this by not using append
	mb.byteBuffer = append(mb.byteBuffer, content...)
	mb.byteBuffer = append(mb.byteBuffer, ',')
}
