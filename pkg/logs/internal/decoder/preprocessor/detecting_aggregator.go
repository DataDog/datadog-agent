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
//
// When isDefaultPath is true, it also collects COAT telemetry that simulates what the
// combining aggregator would do, tracking lines that would be combined and groups that
// would be truncated if auto multiline were enabled by default.
type detectingAggregator struct {
	collected             []AggregatedMessageWithTokens
	previousMsg           *message.Message
	previousMsgTokens     []Token
	previousWasStartGroup bool
	multiLineMatchInfo    *status.CountInfo
	shouldTruncate        bool
	maxContentSize        int
	tagTruncatedLogs      bool
	// COAT simulation state
	isDefaultPath       bool
	simulatedBufLen     int
	linesInCurrentGroup int
	inGroup             bool
}

// NewDetectingAggregator creates a new detecting aggregator.
// maxContentSize is used both for truncation handling and to simulate truncation
// detection for COAT telemetry.
func NewDetectingAggregator(tailerInfo *status.InfoRegistry, maxContentSize int, tagTruncatedLogs bool, isDefaultPath bool) Aggregator {
	multiLineMatchInfo := status.NewCountInfo("MultiLine matches")
	tailerInfo.Register(multiLineMatchInfo)

	return &detectingAggregator{
		multiLineMatchInfo: multiLineMatchInfo,
		maxContentSize:     maxContentSize,
		tagTruncatedLogs:   tagTruncatedLogs,
		isDefaultPath:      isDefaultPath,
	}
}

// Process processes a message with a label and returns any emitted messages.
func (d *detectingAggregator) Process(msg *message.Message, label Label, tokens []Token) []AggregatedMessageWithTokens {
	d.collected = d.collected[:0]

	if d.isDefaultPath {
		metrics.TlmAutoMultilineTotalLines.Inc()
	}

	// Handle aggregate label
	if label == aggregate {
		if d.previousMsg != nil && d.previousWasStartGroup {
			// Tag the previous message as start of multiline group
			d.previousMsg.ParsingExtra.Tags = append(d.previousMsg.ParsingExtra.Tags, "auto_multiline_detected:true")
			d.emit(d.previousMsg, d.previousMsgTokens)
			// Track that we detected and tagged a multiline log
			metrics.TlmAutoMultilineAggregatorFlush.Inc("false", "auto_multi_line_detected")
			d.previousMsg = nil
			d.previousMsgTokens = nil
			d.previousWasStartGroup = false
		} else if d.previousMsg != nil {
			// Previous message wasn't a startGroup, so just output it without tags
			d.emit(d.previousMsg, d.previousMsgTokens)
			d.previousMsg = nil
			d.previousMsgTokens = nil
			d.previousWasStartGroup = false
		}
		// Output the current aggregate message immediately
		d.emit(msg, tokens)
		d.processSimulatedAggregate(msg)
		return d.collected
	}

	// Handle noAggregate label: output immediately without tags
	if label == noAggregate {
		// Flush any pending previous message first
		if d.previousMsg != nil {
			d.emit(d.previousMsg, d.previousMsgTokens)
			d.previousMsg = nil
			d.previousMsgTokens = nil
			d.previousWasStartGroup = false
		}
		d.emit(msg, tokens)
		d.resetSimulatedGroup()
		return d.collected
	}

	// Handle startGroup: flush previous and store current
	if label == startGroup {
		if d.previousMsg != nil {
			d.emit(d.previousMsg, d.previousMsgTokens)
		}
		d.multiLineMatchInfo.Add(1)
		d.previousMsg = msg
		d.previousMsgTokens = tokens
		d.previousWasStartGroup = true
		d.processSimulatedStartGroup(msg)
		return d.collected
	}

	return d.collected
}

// Flush returns any pending message (called on handler flush).
func (d *detectingAggregator) Flush() []AggregatedMessageWithTokens {
	d.collected = d.collected[:0]
	if d.previousMsg != nil {
		d.emit(d.previousMsg, d.previousMsgTokens)
		d.previousMsg = nil
		d.previousMsgTokens = nil
		d.previousWasStartGroup = false
	}
	d.resetSimulatedGroup()
	return d.collected
}

// IsEmpty returns true if there's no pending message.
func (d *detectingAggregator) IsEmpty() bool {
	return d.previousMsg == nil
}

// processSimulatedStartGroup handles the startGroup transition for COAT simulation.
func (d *detectingAggregator) processSimulatedStartGroup(msg *message.Message) {
	if !d.isDefaultPath {
		return
	}
	// Finalize any previous group (no truncation on normal finalize)
	d.resetSimulatedGroup()

	// A startGroup that is already >= maxContentSize would be flushed immediately
	// by the combining aggregator. That's a single oversized line -- excluded from
	// our truncation metric since it would be truncated regardless.
	if msg.RawDataLen >= d.maxContentSize {
		return
	}

	d.inGroup = true
	d.simulatedBufLen = len(msg.GetContent())
	d.linesInCurrentGroup = 1
}

// processSimulatedAggregate handles the aggregate transition for COAT simulation.
func (d *detectingAggregator) processSimulatedAggregate(msg *message.Message) {
	if !d.isDefaultPath {
		return
	}

	if !d.inGroup {
		return
	}

	// This line would be combined in combining mode
	metrics.TlmAutoMultilineWouldCombine.Inc()

	// When the first aggregate arrives, also count the startGroup line that anchors this group
	if d.linesInCurrentGroup == 1 {
		metrics.TlmAutoMultilineWouldCombine.Inc()
	}

	// Simulate the combining aggregator's overflow check:
	// if msg.RawDataLen + bucket.buffer.Len() >= maxContentSize → truncation
	if msg.RawDataLen+d.simulatedBufLen >= d.maxContentSize {
		truncatedLines := float64(d.linesInCurrentGroup + 1)
		metrics.TlmAutoMultilineWouldTruncate.Add(truncatedLines)
		d.resetSimulatedGroup()
		return
	}

	// len(EscapedLineFeed) == 2
	d.simulatedBufLen += len(message.EscapedLineFeed) + len(msg.GetContent())
	d.linesInCurrentGroup++
}

// resetSimulatedGroup resets the COAT simulation state.
func (d *detectingAggregator) resetSimulatedGroup() {
	d.inGroup = false
	d.simulatedBufLen = 0
	d.linesInCurrentGroup = 0
}

func (d *detectingAggregator) emit(msg *message.Message, tokens []Token) {
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
	d.collected = append(d.collected, AggregatedMessageWithTokens{Msg: msg, Tokens: tokens})
}
