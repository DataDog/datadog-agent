// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// SingleLineHandler takes care of tracking the line length
// and truncating them when they are too long.
type SingleLineHandler struct {
	outputFn       func(*message.Message)
	shouldTruncate bool
	lineLimit      int
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

func addTruncatedTag(msg *message.Message) {
	if pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs") {
		msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))
	}
}

// process transforms a raw line into a structured line,
// it guarantees that the content of the line won't exceed
// the limit and that the length of the line is properly tracked
// so that the agent restarts tailing from the right place.
func (h *SingleLineHandler) process(msg *message.Message) {
	lastWasTruncated := h.shouldTruncate
	content := msg.GetContent()
	h.shouldTruncate = len(content) >= h.lineLimit

	content = bytes.TrimSpace(content)

	if lastWasTruncated {
		// the previous line has been truncated because it was too long,
		// the new line is just a remainder,
		// adding the truncated flag at the beginning of the content
		content = append(message.TruncatedFlag, content...)
		addTruncatedTag(msg)
	}

	// how should we detect logs which are too long before rendering them?
	if h.shouldTruncate {
		// the line is too long, it needs to be cut off and send,
		// adding the truncated flag the end of the content
		content = append(content, message.TruncatedFlag...)
		addTruncatedTag(msg)
	}

	msg.SetContent(content) // refresh the content in the message
	h.outputFn(msg)
}
