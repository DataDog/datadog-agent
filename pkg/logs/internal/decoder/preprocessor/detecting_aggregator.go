// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// detectingAggregator detects multiline groups and tags the start line without aggregating.
// It outputs messages immediately for performance.
type detectingAggregator struct {
	collected             []*message.Message
	previousMsg           *message.Message
	previousWasStartGroup bool
	multiLineMatchInfo    *status.CountInfo
	shouldTruncate        bool
	maxContentSize        int
	tagTruncatedLogs      bool
}

// NewDetectingAggregator creates a new detecting aggregator.
func NewDetectingAggregator(tailerInfo *status.InfoRegistry, maxContentSize int, tagTruncatedLogs bool) Aggregator {
	multiLineMatchInfo := status.NewCountInfo("MultiLine matches")
	tailerInfo.Register(multiLineMatchInfo)

	return &detectingAggregator{
		multiLineMatchInfo: multiLineMatchInfo,
		maxContentSize:     maxContentSize,
		tagTruncatedLogs:   tagTruncatedLogs,
	}
}

// Process processes a message with a label and returns any emitted messages.
func (d *detectingAggregator) Process(msg *message.Message, label Label) []*message.Message {
	d.collected = d.collected[:0]

	// Handle aggregate label
	if label == aggregate {
		if d.previousMsg != nil && d.previousWasStartGroup {
			// Tag the previous message as start of multiline group
			d.previousMsg.ParsingExtra.Tags = append(d.previousMsg.ParsingExtra.Tags, "auto_multiline_detected:true")
			d.emit(d.previousMsg)
			// Track that we detected and tagged a multiline log
			metrics.TlmAutoMultilineAggregatorFlush.Inc("false", "auto_multi_line_detected")
			d.previousMsg = nil
			d.previousWasStartGroup = false
		} else if d.previousMsg != nil {
			// Previous message wasn't a startGroup, so just output it without tags
			d.emit(d.previousMsg)
			d.previousMsg = nil
			d.previousWasStartGroup = false
		}
		// Output the current aggregate message immediately
		d.emit(msg)
		return d.collected
	}

	// Handle noAggregate label: output immediately without tags
	if label == noAggregate {
		// Flush any pending previous message first
		if d.previousMsg != nil {
			d.emit(d.previousMsg)
			d.previousMsg = nil
			d.previousWasStartGroup = false
		}
		d.emit(msg)
		return d.collected
	}

	// Handle startGroup: flush previous and store current
	if label == startGroup {
		if d.previousMsg != nil {
			d.emit(d.previousMsg)
		}
		d.multiLineMatchInfo.Add(1)
		d.previousMsg = msg
		d.previousWasStartGroup = true
		return d.collected
	}

	return d.collected
}

// Flush returns any pending message (called on handler flush).
func (d *detectingAggregator) Flush() []*message.Message {
	d.collected = d.collected[:0]
	if d.previousMsg != nil {
		d.emit(d.previousMsg)
		d.previousMsg = nil
		d.previousWasStartGroup = false
	}
	return d.collected
}

// IsEmpty returns true if there's no pending message.
func (d *detectingAggregator) IsEmpty() bool {
	return d.previousMsg == nil
}

func (d *detectingAggregator) emit(msg *message.Message) {
	lastWasTruncated := d.shouldTruncate
	content := msg.GetContent()
	d.shouldTruncate = len(content) > d.maxContentSize || msg.ParsingExtra.IsTruncated

	if lastWasTruncated {
		content = append(message.TruncatedFlag, content...)
	}

	if d.shouldTruncate {
		content = append(content, message.TruncatedFlag...)
		metrics.LogsTruncated.Add(1)
	}

	if lastWasTruncated || d.shouldTruncate {
		msg.ParsingExtra.IsTruncated = true
		if d.tagTruncatedLogs {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag("single_line"))
		}
	}

	msg.SetContent(content)
	d.collected = append(d.collected, msg)
}
