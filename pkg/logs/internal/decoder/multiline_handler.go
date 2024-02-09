// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// Currently only reported when telemetryEnabled is true. telemetryEnabled is only true when
// the multi_line handler is used with auto_multi_line_detection enabled.
const linesCombinedTelemetryMetricName = "datadog.logs_agent.auto_multi_line_lines_combined"

// MultiLineHandler makes sure that multiple lines from a same content
// are properly put together.
type MultiLineHandler struct {
	outputFn          func(*message.Message)
	newContentRe      *regexp.Regexp
	buffer            *bytes.Buffer
	flushTimeout      time.Duration
	flushTimer        *time.Timer
	lineLimit         int
	shouldTruncate    bool
	linesLen          int
	status            string
	timestamp         string
	countInfo         *status.CountInfo
	linesCombinedInfo *status.CountInfo
	telemetryEnabled  bool
	linesCombined     int
}

// NewMultiLineHandler returns a new MultiLineHandler.
func NewMultiLineHandler(outputFn func(*message.Message), newContentRe *regexp.Regexp, flushTimeout time.Duration, lineLimit int, telemetryEnabled bool) *MultiLineHandler {
	return &MultiLineHandler{
		outputFn:          outputFn,
		newContentRe:      newContentRe,
		buffer:            bytes.NewBuffer(nil),
		flushTimeout:      flushTimeout,
		lineLimit:         lineLimit,
		countInfo:         status.NewCountInfo("MultiLine matches"),
		linesCombinedInfo: status.NewCountInfo("Lines Combined"),
		telemetryEnabled:  telemetryEnabled,
		linesCombined:     0,
	}
}

func (h *MultiLineHandler) flushChan() <-chan time.Time {
	if h.flushTimer != nil && h.buffer.Len() > 0 {
		return h.flushTimer.C
	}
	return nil
}

func (h *MultiLineHandler) flush() {
	h.sendBuffer()
}

// process aggregates multiple lines to form a full multiline message,
// it stops when a line matches with the new content regular expression.
// It also makes sure that the content will never exceed the limit
// and that the length of the lines is properly tracked
// so that the agent restarts tailing from the right place.
func (h *MultiLineHandler) process(message *message.Message) {
	if h.flushTimer != nil && h.buffer.Len() > 0 {
		// stop the flush timer, as we now have data
		if !h.flushTimer.Stop() {
			<-h.flushTimer.C
		}
	}

	if h.newContentRe.Match(message.GetContent()) {
		h.countInfo.Add(1)
		// the current line is part of a new message,
		// send the buffer
		h.sendBuffer()
	}

	isTruncated := h.shouldTruncate
	h.shouldTruncate = false

	// track the raw data length and the timestamp so that the agent tails
	// from the right place at restart
	h.linesLen += message.RawDataLen
	h.timestamp = message.ParsingExtra.Timestamp
	h.status = message.Status
	h.linesCombined++

	if h.buffer.Len() > 0 {
		// the buffer already contains some data which means that
		// the current line is not the first line of the message
		h.buffer.Write(escapedLineFeed)
	}

	if isTruncated {
		// the previous line has been truncated because it was too long,
		// the new line is just a remainder,
		// adding the truncated flag at the beginning of the content
		h.buffer.Write(truncatedFlag)
	}

	h.buffer.Write(message.GetContent())

	if h.buffer.Len() >= h.lineLimit {
		// the multiline message is too long, it needs to be cut off and send,
		// adding the truncated flag the end of the content
		h.buffer.Write(truncatedFlag)
		h.sendBuffer()
		h.shouldTruncate = true
	}

	if h.buffer.Len() > 0 {
		// since there's buffered data, start the flush timer to flush it
		if h.flushTimer == nil {
			h.flushTimer = time.NewTimer(h.flushTimeout)
		} else {
			h.flushTimer.Reset(h.flushTimeout)
		}
	}
}

// sendBuffer forwards the content stored in the buffer
// to the output function.
func (h *MultiLineHandler) sendBuffer() {
	defer func() {
		h.buffer.Reset()
		h.linesLen = 0
		h.linesCombined = 0
		h.shouldTruncate = false
	}()

	data := bytes.TrimSpace(h.buffer.Bytes())
	content := make([]byte, len(data))
	copy(content, data)

	if len(content) > 0 || h.linesLen > 0 {
		if h.linesCombined > 0 {
			// -1 to ignore the line matching the pattern. This leave a count of only combined lines.
			linesCombined := int64(h.linesCombined - 1)
			h.linesCombinedInfo.Add(linesCombined)
			if h.telemetryEnabled {
				telemetry.GetStatsTelemetryProvider().Count(linesCombinedTelemetryMetricName, float64(linesCombined), []string{})
			}
		}

		h.outputFn(NewMessage(content, h.status, h.linesLen, h.timestamp))
	}
}
