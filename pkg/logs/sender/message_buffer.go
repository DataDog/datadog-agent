// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MessageBuffer accumulates lines in buffer escaping all '\n'
// and accumulates the total number of bytes of all lines in line representation (line + '\n') in rawDataLen
type MessageBuffer struct {
	maxBatchCount  int
	maxRequestSize int
	messageBuffer  []*message.Message
	byteBuffer     []byte
}

// NewMessageBuffer returns a new MessageBuffer
func NewMessageBuffer(maxBatchCount, maxRequestSize int) *MessageBuffer {
	return &MessageBuffer{
		maxBatchCount:  maxBatchCount,
		maxRequestSize: maxRequestSize,
		messageBuffer:  make([]*message.Message, 0, maxBatchCount),
		byteBuffer:     make([]byte, 1, maxRequestSize),
	}
}

func (mb *MessageBuffer) tryAppendToBuffer(m *message.Message) (bool, bool) {

	sendNow, appendAgain := false, false

	if (len(mb.messageBuffer) < cap(mb.messageBuffer)) && mb.hasSpaceInByteBuffer(m.Content) {
		// fits. append the message
		mb.messageBuffer = append(mb.messageBuffer, m)
		mb.appendByteBuffer(m.Content)
	} else {
		// doesn't fit, which can only be caused by not enough space in byteBiffer
		// signal to send immediately and append again
		sendNow, appendAgain = true, true
	}

	// if after append, we reach the batchsize, we should send now
	if len(mb.messageBuffer) == cap(mb.messageBuffer) {
		sendNow = true
	}

	return sendNow, appendAgain
}

func (mb *MessageBuffer) isEmpty() bool {
	return len(mb.messageBuffer) == 0
}

func (mb *MessageBuffer) clear() {
	mb.messageBuffer = mb.messageBuffer[:0]
	mb.byteBuffer = mb.byteBuffer[:1] // keep the first byte, it's used for : '['
}

func (mb *MessageBuffer) hasSpaceInByteBuffer(content []byte) bool {
	return len(mb.byteBuffer)+len(content)+1 < cap(mb.byteBuffer)
}

func (mb *MessageBuffer) appendByteBuffer(content []byte) {
	// increase the slice length, TODO can optimized this by not using append
	mb.byteBuffer = append(mb.byteBuffer, content...)
	mb.byteBuffer = append(mb.byteBuffer, ',')
}

func (mb *MessageBuffer) getByteBuffer() []byte {
	// here we write the json '[' and ']'
	mb.byteBuffer[0] = '['
	mb.byteBuffer[len(mb.byteBuffer)-1] = ']'
	return mb.byteBuffer
}
