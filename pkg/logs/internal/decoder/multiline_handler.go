// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"bytes"
	"regexp"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
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
	isBufferTruncated bool
	linesLen          int
	msg               *message.Message
	countInfo         *status.CountInfo
	linesCombinedInfo *status.CountInfo
	telemetryEnabled  bool
	linesCombined     int
	multiLineTagValue string
}

// NewMultiLineHandler returns a new MultiLineHandler.
func NewMultiLineHandler(outputFn func(*message.Message), newContentRe *regexp.Regexp, flushTimeout time.Duration, lineLimit int, telemetryEnabled bool, tailerInfo *status.InfoRegistry, multiLineTagValue string) *MultiLineHandler {

	i := status.NewMappedInfo("Multi-Line Pattern")
	i.SetMessage("Pattern", newContentRe.String())
	tailerInfo.Register(i)

	h := &MultiLineHandler{
		outputFn:          outputFn,
		newContentRe:      newContentRe,
		buffer:            bytes.NewBuffer(nil),
		flushTimeout:      flushTimeout,
		lineLimit:         lineLimit,
		countInfo:         status.NewCountInfo("MultiLine matches"),
		linesCombinedInfo: status.NewCountInfo("Lines Combined"),
		telemetryEnabled:  telemetryEnabled,
		linesCombined:     0,
		multiLineTagValue: multiLineTagValue,
	}
	return h
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
func (h *MultiLineHandler) process(msg *message.Message) {
	if h.flushTimer != nil && h.buffer.Len() > 0 {
		// stop the flush timer, as we now have data
		if !h.flushTimer.Stop() {
			<-h.flushTimer.C
		}
	}

	if h.newContentRe.Match(msg.GetContent()) {
		h.countInfo.Add(1)
		// the current line is part of a new message,
		// send the buffer
		h.sendBuffer()
	}

	isTruncated := h.shouldTruncate
	h.shouldTruncate = false

	// track the raw data length so that the agent tails
	// from the right place at restart
	h.linesLen += msg.RawDataLen
	h.msg = msg
	h.linesCombined++

	if h.buffer.Len() > 0 {
		// the buffer already contains some data which means that
		// the current line is not the first line of the message
		h.buffer.Write(message.EscapedLineFeed)
	}

	if isTruncated {
		// the previous line has been truncated because it was too long,
		// the new line is just a remainder,
		// adding the truncated flag at the beginning of the content
		h.buffer.Write(message.TruncatedFlag)
		h.isBufferTruncated = true
	}

	h.buffer.Write(msg.GetContent())

	if h.buffer.Len() >= h.lineLimit {
		// the multiline message is too long, it needs to be cut off and send,
		// adding the truncated flag the end of the content
		h.buffer.Write(message.TruncatedFlag)
		h.isBufferTruncated = true
		h.sendBuffer()
		h.shouldTruncate = true
		metrics.LogsTruncated.Add(1)
		if msg == nil || msg.Origin == nil {
			metrics.TlmTruncatedCount.Inc("", "")
		} else {
			metrics.TlmTruncatedCount.Inc(msg.Origin.Service(), msg.Origin.Source())
		}
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
		h.isBufferTruncated = false
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

		msg := h.msg
		msg.SetContent(content)
		msg.RawDataLen = h.linesLen
		msg.ParsingExtra.IsTruncated = h.isBufferTruncated

		tlmTags := []string{"false", "single_line"}
		if h.isBufferTruncated {
			tlmTags[0] = "true"
			if pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs") {
				msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("multiline_regex"))
			}
		}
		if h.linesCombined > 1 {
			tlmTags[1] = h.multiLineTagValue
			if pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs") {
				msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.MultiLineSourceTag(h.multiLineTagValue))
			}
		}
		metrics.TlmAutoMultilineAggregatorFlush.Inc(tlmTags...)
		h.outputFn(msg)
	}
}
