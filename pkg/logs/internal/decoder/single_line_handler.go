// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// SingleLineHandler takes care of tracking the line length
// and truncating them when they are too long.
type SingleLineHandler struct {
	outputFn       func(*message.Message)
	shouldTruncate bool
	lineLimit      int
	buffer         []byte // buffer to avoid reallocs in `process`
}

// NewSingleLineHandler returns a new SingleLineHandler.
func NewSingleLineHandler(outputFn func(*message.Message), lineLimit int) *SingleLineHandler {
	return &SingleLineHandler{
		outputFn:  outputFn,
		lineLimit: lineLimit,
	}
}

func (h *SingleLineHandler) flushChan() <-chan time.Time {
	return nil
}

func (h *SingleLineHandler) flush() {
	// do nothing
}

// process transforms a raw line into a structured line, it guarantees that the
// content of the line won't exceed the limit and that the length of the line is
// properly tracked so that the agent restarts tailing from the right place.
func (h *SingleLineHandler) process(message *message.Message) {
	isTruncated := h.shouldTruncate
	h.shouldTruncate = false

	message.Content = bytes.TrimSpace(message.Content)

	h.buffer = h.buffer[:0] // reset, avoid polluting different process calls

	if isTruncated {
		// the previous line has been truncated because it was too long,
		// the new line is just a remainder, adding the truncated flag
		// at the beginning of the content
		h.buffer = append(h.buffer, truncatedFlag...)
		h.buffer = append(h.buffer, message.Content...)
		message.Content = h.buffer
	}

	if len(message.Content) >= h.lineLimit {
		if !isTruncated {
			// The line is too long but we're not dealing with the
			// truncated side.
			h.buffer = append(h.buffer, message.Content...)
		}
		// the line is too long, it needs to be cut off and send, adding
		// the truncated flag the end of the content
		h.buffer = append(h.buffer, truncatedFlag...)
		message.Content = h.buffer
		// make sure the following part of the line will be cut off as
		// well
		h.shouldTruncate = true
	}
	h.outputFn(message)
}
