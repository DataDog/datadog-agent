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
	messageBuffer    []*message.Message
	contentSize      int
	contentSizeLimit int
}

// NewMessageBuffer returns a new MessageBuffer.
func NewMessageBuffer(batchSizeLimit int, contentSizeLimit int) *MessageBuffer {
	return &MessageBuffer{
		messageBuffer:    make([]*message.Message, 0, batchSizeLimit),
		contentSizeLimit: contentSizeLimit,
	}
}

// AddMessage adds a message to the buffer if there is still some free space,
// returns true if the message was added.
func (p *MessageBuffer) AddMessage(message *message.Message) bool {
	contentSize := len(message.GetContent())
	if len(p.messageBuffer) < cap(p.messageBuffer) && p.contentSize+contentSize <= p.contentSizeLimit {
		p.messageBuffer = append(p.messageBuffer, message)
		p.contentSize += contentSize
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
	return len(p.messageBuffer) == cap(p.messageBuffer) || p.contentSize == p.contentSizeLimit
}

// IsEmpty returns true if the buffer is empty.
func (p *MessageBuffer) IsEmpty() bool {
	return len(p.messageBuffer) == 0
}

// ContentSizeLimit returns the configured content size limit. Messages above this limit are not accepted.
func (p *MessageBuffer) ContentSizeLimit() int {
	return p.contentSizeLimit
}
