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

// RegexCombiner aggregates log lines into multiline messages using a regular expression
// to identify the start of a new log entry. It is the pull-model equivalent of the
// decoder's MultiLineHandler. The flush timer is managed externally by the Pipeline.
type RegexCombiner struct {
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

// NewRegexCombiner returns a new RegexCombiner.
func NewRegexCombiner(newContentRe *regexp.Regexp, lineLimit int, telemetryEnabled bool, tailerInfo *status.InfoRegistry, multiLineTagValue string) *RegexCombiner {
	i := status.NewMappedInfo("Multi-Line Pattern")
	i.SetMessage("Pattern", newContentRe.String())
	tailerInfo.Register(i)

	return &RegexCombiner{
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
func (c *RegexCombiner) CountInfo() *status.CountInfo {
	return c.countInfo
}

// LinesCombinedInfo returns the counter tracking lines combined into multiline messages.
// Used by the decoder to sync shared counters across multiple tailers for the same source.
func (c *RegexCombiner) LinesCombinedInfo() *status.CountInfo {
	return c.linesCombinedInfo
}

// SetCountInfo replaces the multiline match counter (used by decoder.syncSourceInfo).
func (c *RegexCombiner) SetCountInfo(info *status.CountInfo) {
	c.countInfo = info
}

// SetLinesCombinedInfo replaces the lines-combined counter (used by decoder.syncSourceInfo).
func (c *RegexCombiner) SetLinesCombinedInfo(info *status.CountInfo) {
	c.linesCombinedInfo = info
}

// Process aggregates log lines using the regex to detect new log entry boundaries.
// Returns any completed messages (may be empty if the current line is buffered).
func (c *RegexCombiner) Process(msg *message.Message) []*message.Message {
	c.collected = c.collected[:0]

	if c.newContentRe.Match(msg.GetContent()) {
		c.countInfo.Add(1)
		c.sendBuffer()
	}

	isTruncated := c.shouldTruncate
	c.shouldTruncate = false

	c.linesLen += msg.RawDataLen
	c.msg = msg
	c.linesCombined++

	if c.buffer.Len() > 0 {
		c.buffer.Write(message.EscapedLineFeed)
	}

	if isTruncated {
		c.buffer.Write(message.TruncatedFlag)
		c.isBufferTruncated = true
	}

	c.buffer.Write(msg.GetContent())

	if c.buffer.Len() >= c.lineLimit {
		c.buffer.Write(message.TruncatedFlag)
		c.isBufferTruncated = true
		c.sendBuffer()
		c.shouldTruncate = true
		metrics.LogsTruncated.Add(1)
	}

	return c.collected
}

// Flush returns any buffered content as a completed message and resets state.
func (c *RegexCombiner) Flush() []*message.Message {
	c.collected = c.collected[:0]
	c.sendBuffer()
	return c.collected
}

// IsEmpty returns true if the combiner has no buffered data.
func (c *RegexCombiner) IsEmpty() bool {
	return c.buffer.Len() == 0
}

func (c *RegexCombiner) sendBuffer() {
	defer func() {
		c.buffer.Reset()
		c.linesLen = 0
		c.linesCombined = 0
		c.shouldTruncate = false
		c.isBufferTruncated = false
	}()

	data := bytes.TrimSpace(c.buffer.Bytes())
	content := make([]byte, len(data))
	copy(content, data)

	if len(content) == 0 && c.linesLen == 0 {
		return
	}

	if c.linesCombined > 0 {
		linesCombined := int64(c.linesCombined - 1)
		c.linesCombinedInfo.Add(linesCombined)
		if c.telemetryEnabled {
			telemetry.GetStatsTelemetryProvider().Count(regexLinesCombinedTelemetryMetricName, float64(linesCombined), []string{})
		}
	}

	msg := c.msg
	msg.SetContent(content)
	msg.RawDataLen = c.linesLen
	msg.ParsingExtra.IsTruncated = c.isBufferTruncated

	tlmTags := []string{"false", "single_line"}
	if c.isBufferTruncated {
		tlmTags[0] = "true"
		if pkgconfigsetup.Datadog().GetBool("logs_config.tag_truncated_logs") {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("multiline_regex"))
		}
	}
	if c.linesCombined > 1 {
		tlmTags[1] = c.multiLineTagValue
		if pkgconfigsetup.Datadog().GetBool("logs_config.tag_multi_line_logs") {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.MultiLineSourceTag(c.multiLineTagValue))
		}
	}
	metrics.TlmAutoMultilineAggregatorFlush.Inc(tlmTags...)
	c.collected = append(c.collected, msg)
}
