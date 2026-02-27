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
	collected             []CompletedMessage
	previousMsg           *message.Message
	previousMsgTokens     []Token
	previousWasStartGroup bool
	multiLineMatchInfo    *status.CountInfo
}

// NewDetectingAggregator creates a new detecting aggregator.
func NewDetectingAggregator(tailerInfo *status.InfoRegistry) Aggregator {
	multiLineMatchInfo := status.NewCountInfo("MultiLine matches")
	tailerInfo.Register(multiLineMatchInfo)

	return &detectingAggregator{
		multiLineMatchInfo: multiLineMatchInfo,
	}
}

// Process processes a message with a label and returns any emitted messages.
func (d *detectingAggregator) Process(msg *message.Message, label Label, tokens []Token) []CompletedMessage {
	d.collected = d.collected[:0]

	// Handle aggregate label
	if label == aggregate {
		if d.previousMsg != nil && d.previousWasStartGroup {
			// Tag the previous message as start of multiline group
			d.previousMsg.ParsingExtra.Tags = append(d.previousMsg.ParsingExtra.Tags, "auto_multiline_detected:true")
			d.collected = append(d.collected, CompletedMessage{Msg: d.previousMsg, Tokens: d.previousMsgTokens})
			// Track that we detected and tagged a multiline log
			metrics.TlmAutoMultilineAggregatorFlush.Inc("false", "auto_multi_line_detected")
			d.previousMsg = nil
			d.previousMsgTokens = nil
			d.previousWasStartGroup = false
		} else if d.previousMsg != nil {
			// Previous message wasn't a startGroup, so just output it without tags
			d.collected = append(d.collected, CompletedMessage{Msg: d.previousMsg, Tokens: d.previousMsgTokens})
			d.previousMsg = nil
			d.previousMsgTokens = nil
			d.previousWasStartGroup = false
		}
		// Output the current aggregate message immediately
		d.collected = append(d.collected, CompletedMessage{Msg: msg, Tokens: tokens})
		return d.collected
	}

	// Handle noAggregate label: output immediately without tags
	if label == noAggregate {
		// Flush any pending previous message first
		if d.previousMsg != nil {
			d.collected = append(d.collected, CompletedMessage{Msg: d.previousMsg, Tokens: d.previousMsgTokens})
			d.previousMsg = nil
			d.previousMsgTokens = nil
			d.previousWasStartGroup = false
		}
		d.collected = append(d.collected, CompletedMessage{Msg: msg, Tokens: tokens})
		return d.collected
	}

	// Handle startGroup: flush previous and store current
	if label == startGroup {
		if d.previousMsg != nil {
			d.collected = append(d.collected, CompletedMessage{Msg: d.previousMsg, Tokens: d.previousMsgTokens})
		}
		d.multiLineMatchInfo.Add(1)
		d.previousMsg = msg
		d.previousMsgTokens = tokens
		d.previousWasStartGroup = true
		return d.collected
	}

	return d.collected
}

// Flush returns any pending message (called on handler flush).
func (d *detectingAggregator) Flush() []CompletedMessage {
	d.collected = d.collected[:0]
	if d.previousMsg != nil {
		d.collected = append(d.collected, CompletedMessage{Msg: d.previousMsg, Tokens: d.previousMsgTokens})
		d.previousMsg = nil
		d.previousMsgTokens = nil
		d.previousWasStartGroup = false
	}
	return d.collected
}

// IsEmpty returns true if there's no pending message.
func (d *detectingAggregator) IsEmpty() bool {
	return d.previousMsg == nil
}
