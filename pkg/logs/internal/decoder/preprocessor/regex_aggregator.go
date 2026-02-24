// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"bytes"
	"regexp"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const regexLinesCombinedTelemetryMetricName = "datadog.logs_agent.auto_multi_line_lines_combined"

// RegexAggregator aggregates log lines into multiline messages using a regular expression
// to identify the start of a new log entry. It is the equivalent of the decoder's MultiLineHandler.
// The flush timer is managed externally by the Preprocessor.
type RegexAggregator struct {
	newContentRe      *regexp.Regexp
	buffer            *bytes.Buffer
	lineLimit         int
	shouldTruncate    bool
	isBufferTruncated bool
	linesLen          int
	msg               *message.Message
	linesCombined     int
	telemetryEnabled  bool
	multiLineTagValue string
	countInfo         *status.CountInfo
	linesCombinedInfo *status.CountInfo
	collected         []*message.Message
}

// NewRegexAggregator returns a new RegexAggregator.
func NewRegexAggregator(newContentRe *regexp.Regexp, lineLimit int, telemetryEnabled bool, tailerInfo *status.InfoRegistry, multiLineTagValue string) *RegexAggregator {
	i := status.NewMappedInfo("Multi-Line Pattern")
	i.SetMessage("Pattern", newContentRe.String())
	tailerInfo.Register(i)

	return &RegexAggregator{
		newContentRe:      newContentRe,
		buffer:            bytes.NewBuffer(nil),
		lineLimit:         lineLimit,
		telemetryEnabled:  telemetryEnabled,
		multiLineTagValue: multiLineTagValue,
		countInfo:         status.NewCountInfo("MultiLine matches"),
		linesCombinedInfo: status.NewCountInfo("Lines Combined"),
		collected:         make([]*message.Message, 0, 1),
	}
}

// CountInfo returns the counter tracking multiline pattern matches.
// Used by the decoder to sync shared counters across multiple tailers for the same source.
func (a *RegexAggregator) CountInfo() *status.CountInfo {
	return a.countInfo
}

// LinesCombinedInfo returns the counter tracking lines combined into multiline messages.
// Used by the decoder to sync shared counters across multiple tailers for the same source.
func (a *RegexAggregator) LinesCombinedInfo() *status.CountInfo {
	return a.linesCombinedInfo
}

// SetCountInfo replaces the multiline match counter (used by decoder.syncSourceInfo).
func (a *RegexAggregator) SetCountInfo(info *status.CountInfo) {
	a.countInfo = info
}

// SetLinesCombinedInfo replaces the lines-combined counter (used by decoder.syncSourceInfo).
func (a *RegexAggregator) SetLinesCombinedInfo(info *status.CountInfo) {
	a.linesCombinedInfo = info
}

// Process aggregates log lines using the regex to detect new log entry boundaries.
// Returns any completed messages (may be empty if the current line is buffered). label is unused.
func (a *RegexAggregator) Process(msg *message.Message, _ Label) []*message.Message {
	a.collected = a.collected[:0]

	if a.newContentRe.Match(msg.GetContent()) {
		a.countInfo.Add(1)
		a.sendBuffer()
	}

	isTruncated := a.shouldTruncate
	a.shouldTruncate = false

	a.linesLen += msg.RawDataLen
	a.msg = msg
	a.linesCombined++

	if a.buffer.Len() > 0 {
		a.buffer.Write(message.EscapedLineFeed)
	}

	if isTruncated {
		a.buffer.Write(message.TruncatedFlag)
		a.isBufferTruncated = true
	}

	a.buffer.Write(msg.GetContent())

	if a.buffer.Len() >= a.lineLimit {
		a.buffer.Write(message.TruncatedFlag)
		a.isBufferTruncated = true
		a.sendBuffer()
		a.shouldTruncate = true
		metrics.LogsTruncated.Add(1)
	}

	return a.collected
}

// Flush returns any buffered content as a completed message and resets state.
func (a *RegexAggregator) Flush() []*message.Message {
	a.collected = a.collected[:0]
	a.sendBuffer()
	return a.collected
}

// IsEmpty returns true if the aggregator has no buffered data.
func (a *RegexAggregator) IsEmpty() bool {
	return a.buffer.Len() == 0
}

func (a *RegexAggregator) sendBuffer() {
	defer func() {
		a.buffer.Reset()
		a.linesLen = 0
		a.linesCombined = 0
		a.shouldTruncate = false
		a.isBufferTruncated = false
	}()

	data := bytes.TrimSpace(a.buffer.Bytes())
	content := make([]byte, len(data))
	copy(content, data)

	if len(content) == 0 && a.linesLen == 0 {
		return
	}

	if a.linesCombined > 0 {
		linesCombined := int64(a.linesCombined - 1)
		a.linesCombinedInfo.Add(linesCombined)
		if a.telemetryEnabled {
			telemetry.GetStatsTelemetryProvider().Count(regexLinesCombinedTelemetryMetricName, float64(linesCombined), []string{})
		}
	}

	msg := a.msg
	msg.SetContent(content)
	msg.RawDataLen = a.linesLen
	msg.ParsingExtra.IsTruncated = a.isBufferTruncated

	tlmTags := []string{"false", "single_line"}
	if a.isBufferTruncated {
		tlmTags[0] = "true"
		if pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs") {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("multiline_regex"))
		}
	}
	if a.linesCombined > 1 {
		tlmTags[1] = a.multiLineTagValue
		if pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs") {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.MultiLineSourceTag(a.multiLineTagValue))
		}
	}
	metrics.TlmAutoMultilineAggregatorFlush.Inc(tlmTags...)
	a.collected = append(a.collected, msg)
}
