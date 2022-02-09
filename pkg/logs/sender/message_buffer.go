// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MessageBuffer accumulates messages to a buffer until the max capacity is reached.
type MessageBuffer struct {
	messageBuffer          []*message.Message
	contentSize            int
	singleMessageSizeLimit int
	bufferSizeLimit        int
}

// NewMessageBuffer returns a new MessageBuffer.
func NewMessageBuffer(batchSizeLimit int, singleMessageSizeLimit int, bufferSizeLimit int) *MessageBuffer {
	return &MessageBuffer{
		messageBuffer:          make([]*message.Message, 0, batchSizeLimit),
		singleMessageSizeLimit: singleMessageSizeLimit,
		bufferSizeLimit:        bufferSizeLimit,
	}
}

// AddMessage adds a message to the buffer if there is still some free space,
// returns true if the message was added.
func (p *MessageBuffer) AddMessage(message *message.Message) bool {
	newMessageSize := len(message.Content)
	if newMessageSize > p.singleMessageSizeLimit {
		return false
	}

	if len(p.messageBuffer) < cap(p.messageBuffer) && p.contentSize+newMessageSize <= p.bufferSizeLimit {
		p.messageBuffer = append(p.messageBuffer, message)
		p.contentSize += newMessageSize
		return true
	}
	return false
}

// Clear reinitializes the buffer.
func (p *MessageBuffer) Clear() {
	// create a new buffer to avoid race conditions
	p.messageBuffer = make([]*message.Message, 0, cap(p.messageBuffer))
	p.contentSize = 0
}

// GetMessages returns the messages stored in the buffer.
func (p *MessageBuffer) GetMessages() []*message.Message {
	return p.messageBuffer
}

// IsFull returns true if the buffer is full.
func (p *MessageBuffer) IsFull() bool {
	return len(p.messageBuffer) == cap(p.messageBuffer) || p.contentSize == p.bufferSizeLimit
}

// IsEmpty returns true if the buffer is empty.
func (p *MessageBuffer) IsEmpty() bool {
	return len(p.messageBuffer) == 0
}

// SingleMessageSizeLimit returns the configured maximum single message size. Messages above this limit are not accepted.
func (p *MessageBuffer) SingleMessageSizeLimit() int {
	return p.singleMessageSizeLimit
}
