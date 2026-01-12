// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Aggregator is the interface for multiline log processing.
// Both combining and detecting implementations satisfy this interface.
type Aggregator interface {
	Process(msg *message.Message, label Label)
	Flush()
	IsEmpty() bool
}

type bucket struct {
	tagTruncatedLogs bool
	tagMultiLineLogs bool
	maxContentSize   int

	message         *message.Message
	originalDataLen int
	buffer          *bytes.Buffer
	lineCount       int
	shouldTruncate  bool
	needsTruncation bool
}

func (b *bucket) add(msg *message.Message) {
	if b.message == nil {
		b.message = msg
	}
	if b.originalDataLen > 0 {
		b.buffer.Write(message.EscapedLineFeed)
	}
	b.buffer.Write(msg.GetContent())
	b.originalDataLen += msg.RawDataLen
	b.lineCount++
}

func (b *bucket) isEmpty() bool {
	return b.originalDataLen == 0
}

func (b *bucket) reset() {
	b.buffer.Reset()
	b.message = nil
	b.lineCount = 0
	b.originalDataLen = 0
	b.needsTruncation = false
}

func (b *bucket) flush() *message.Message {
	defer b.reset()

	lastWasTruncated := b.shouldTruncate
	b.shouldTruncate = b.buffer.Len() >= b.maxContentSize || b.needsTruncation

	data := bytes.TrimSpace(b.buffer.Bytes())
	content := make([]byte, len(data))
	copy(content, data)

	if lastWasTruncated {
		// The previous line has been truncated because it was too long,
		// the new line is just the remainder. Add the truncated flag at
		// the beginning of the content.
		content = append(message.TruncatedFlag, content...)
	}

	if b.shouldTruncate {
		// The current line is too long. Mark it truncated at the end.
		content = append(content, message.TruncatedFlag...)
		metrics.LogsTruncated.Add(1)
	}

	msg := b.message
	msg.SetContent(content)
	msg.RawDataLen = b.originalDataLen
	tlmTags := []string{"false", "single_line"}

	if b.lineCount > 1 {
		msg.ParsingExtra.IsMultiLine = true
		tlmTags[1] = "auto_multi_line"
		if b.tagMultiLineLogs {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.MultiLineSourceTag("auto_multiline"))
		}
	}

	if lastWasTruncated || b.shouldTruncate {
		msg.ParsingExtra.IsTruncated = true
		tlmTags[0] = "true"
		if b.tagTruncatedLogs {
			if b.lineCount > 1 {
				msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("auto_multiline"))
			} else {
				msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))
			}
		}
	}

	metrics.TlmAutoMultilineAggregatorFlush.Inc(tlmTags...)
	return msg
}

// combiningAggregator aggregates multiline logs with a given label.
type combiningAggregator struct {
	outputFn           func(m *message.Message)
	bucket             *bucket
	maxContentSize     int
	multiLineMatchInfo *status.CountInfo
	linesCombinedInfo  *status.CountInfo
}

// NewCombiningAggregator creates a new combining aggregator.
func NewCombiningAggregator(outputFn func(m *message.Message), maxContentSize int, tagTruncatedLogs bool, tagMultiLineLogs bool, tailerInfo *status.InfoRegistry) Aggregator {
	multiLineMatchInfo := status.NewCountInfo("MultiLine matches")
	linesCombinedInfo := status.NewCountInfo("Lines Combined")
	tailerInfo.Register(multiLineMatchInfo)
	tailerInfo.Register(linesCombinedInfo)

	return &combiningAggregator{
		outputFn: outputFn,
		bucket: &bucket{
			buffer:           bytes.NewBuffer(nil),
			tagTruncatedLogs: tagTruncatedLogs,
			tagMultiLineLogs: tagMultiLineLogs,
			maxContentSize:   maxContentSize,
			lineCount:        0,
			shouldTruncate:   false,
			needsTruncation:  false,
		},
		maxContentSize:     maxContentSize,
		multiLineMatchInfo: multiLineMatchInfo,
		linesCombinedInfo:  linesCombinedInfo,
	}
}

// Process processes a multiline log using a label.
func (a *combiningAggregator) Process(msg *message.Message, label Label) {
	// If `noAggregate` - flush the bucket immediately and then flush the next message.
	if label == noAggregate {
		a.Flush()
		a.bucket.shouldTruncate = false // noAggregate messages should never be truncated at the beginning (Could break JSON formatted messages)
		a.bucket.add(msg)
		a.Flush()
		return
	}

	// If `aggregate` and the bucket is empty - flush the next message.
	if label == aggregate && a.bucket.isEmpty() {
		a.bucket.add(msg)
		a.Flush()
		return
	}

	// If `startGroup` - flush the old bucket to form a new group.
	if label == startGroup {
		a.Flush()
		a.multiLineMatchInfo.Add(1)
		a.bucket.add(msg)
		if msg.RawDataLen >= a.maxContentSize {
			// Start group is too big to append anything to, flush it and reset.
			a.Flush()
		}
		return

	}

	// Check for a total buffer size larger than the limit. This should only be reachable by an aggregate label
	// following a smaller than max-size start group label, and will result in the reset (flush) of the entire bucket.
	// This reset will intentionally break multi-line detection and aggregation for logs larger than the limit, because
	// doing so is safer than assuming we will correctly get a new startGroup for subsequent single line logs.
	if msg.RawDataLen+a.bucket.buffer.Len() >= a.maxContentSize {
		a.bucket.needsTruncation = true
		a.bucket.lineCount++ // Account for the current (not yet processed) message being part of the same log
		a.Flush()

		a.bucket.lineCount++ // Account for the previous (now flushed) message being part of the same log
		a.bucket.add(msg)
		a.Flush()
		return
	}

	// We're an aggregate label within a startGroup and within the maxContentSize. Append new multiline
	a.linesCombinedInfo.Add(1)
	a.bucket.add(msg)
}

// Flush flushes the aggregator.
func (a *combiningAggregator) Flush() {
	if a.bucket.isEmpty() {
		a.bucket.reset()
		return
	}
	a.outputFn(a.bucket.flush())
}

// IsEmpty returns true if the bucket is empty.
func (a *combiningAggregator) IsEmpty() bool {
	return a.bucket.isEmpty()
}

// detectingAggregator detects multiline groups and tags the start line without aggregating.
// It outputs messages immediately for performance.
type detectingAggregator struct {
	outputFn              func(*message.Message)
	previousMsg           *message.Message
	previousWasStartGroup bool
	multiLineMatchInfo    *status.CountInfo
}

// NewDetectingAggregator creates a new detecting aggregator.
func NewDetectingAggregator(outputFn func(*message.Message), tailerInfo *status.InfoRegistry) Aggregator {
	multiLineMatchInfo := status.NewCountInfo("MultiLine matches")
	tailerInfo.Register(multiLineMatchInfo)

	return &detectingAggregator{
		outputFn:           outputFn,
		multiLineMatchInfo: multiLineMatchInfo,
	}
}

// Process processes a message with a label and outputs immediately.
func (d *detectingAggregator) Process(msg *message.Message, label Label) {
	// Handle aggregate label
	if label == aggregate {
		if d.previousMsg != nil && d.previousWasStartGroup {
			// Tag the previous message as start of multiline group
			tag := "auto_multiline_detected:true"
			d.previousMsg.ParsingExtra.Tags = append(d.previousMsg.ParsingExtra.Tags, tag)
			d.outputFn(d.previousMsg)
			// Track that we detected and tagged a multiline log
			metrics.TlmAutoMultilineAggregatorFlush.Inc("false", "auto_multi_line_detected")
			d.previousMsg = nil
			d.previousWasStartGroup = false
		} else if d.previousMsg != nil {
			// Previous message wasn't a startGroup, so just output it without tags
			d.outputFn(d.previousMsg)
			d.previousMsg = nil
			d.previousWasStartGroup = false
		}
		// Output the current aggregate message immediately
		d.outputFn(msg)
		return
	}

	// Handle noAggregate label: output immediately without tags
	if label == noAggregate {
		// Flush any pending previous message first
		if d.previousMsg != nil {
			d.outputFn(d.previousMsg)
			d.previousMsg = nil
			d.previousWasStartGroup = false
		}
		d.outputFn(msg)
		return
	}

	// Handle startGroup: flush previous and store current
	if label == startGroup {
		if d.previousMsg != nil {
			d.outputFn(d.previousMsg)
		}
		d.multiLineMatchInfo.Add(1)
		d.previousMsg = msg
		d.previousWasStartGroup = true
		return
	}
}

// Flush outputs any pending message (called on handler flush).
func (d *detectingAggregator) Flush() {
	if d.previousMsg != nil {
		d.outputFn(d.previousMsg)
		d.previousMsg = nil
		d.previousWasStartGroup = false
	}
}

// IsEmpty returns true if there's no pending message.
func (d *detectingAggregator) IsEmpty() bool {
	return d.previousMsg == nil
}
