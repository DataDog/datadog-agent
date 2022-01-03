// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
)

// SingleLineHandler takes care of tracking the line length
// and truncating them when they are too long.
type SingleLineHandler struct {
	inputChan      chan *Message
	outputChan     chan *Message
	shouldTruncate bool
	lineLimit      int
}

// NewSingleLineHandler returns a new SingleLineHandler.
func NewSingleLineHandler(outputChan chan *Message, lineLimit int) *SingleLineHandler {
	return &SingleLineHandler{
		inputChan:  make(chan *Message),
		outputChan: outputChan,
		lineLimit:  lineLimit,
	}
}

// Handle puts all new lines into a channel for later processing.
func (h *SingleLineHandler) Handle(input *Message) {
	h.inputChan <- input
}

// Stop stops the handler.
func (h *SingleLineHandler) Stop() {
	close(h.inputChan)
}

// Start starts the handler.
func (h *SingleLineHandler) Start() {
	go h.run()
}

// run consumes new lines and processes them.
func (h *SingleLineHandler) run() {
	for line := range h.inputChan {
		h.process(line)
	}
	close(h.outputChan)
}

// process transforms a raw line into a structured line,
// it guarantees that the content of the line won't exceed
// the limit and that the length of the line is properly tracked
// so that the agent restarts tailing from the right place.
func (h *SingleLineHandler) process(message *Message) {
	isTruncated := h.shouldTruncate
	h.shouldTruncate = false

	message.Content = bytes.TrimSpace(message.Content)

	if isTruncated {
		// the previous line has been truncated because it was too long,
		// the new line is just a remainder,
		// adding the truncated flag at the beginning of the content
		message.Content = append(truncatedFlag, message.Content...)
	}

	if len(message.Content) < h.lineLimit {
		h.outputChan <- message
	} else {
		// the line is too long, it needs to be cut off and send,
		// adding the truncated flag the end of the content
		message.Content = append(message.Content, truncatedFlag...)
		h.outputChan <- message
		// make sure the following part of the line will be cut off as well
		h.shouldTruncate = true
	}
}
