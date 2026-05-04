// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// bucket is internal state used by combiningAggregator to accumulate log lines.
type bucket struct {
	tagTruncatedLogs bool
	tagMultiLineLogs bool
	maxContentSize   int

	originalDataLen int
	contentLen      int
	lines           []AggregatedMessageWithTokens
	// shouldTruncate carries truncation state between emitted frames of one oversized
	// single-line log.
	shouldTruncate bool
}

func (b *bucket) add(msg *message.Message, tokens []Token) {
	if b.originalDataLen > 0 {
		b.contentLen += len(message.EscapedLineFeed)
	}
	b.contentLen += len(msg.GetContent())
	b.lines = append(b.lines, AggregatedMessageWithTokens{Msg: msg, Tokens: tokens})
	b.originalDataLen += msg.RawDataLen
}

func (b *bucket) isEmpty() bool {
	return b.originalDataLen == 0
}

func (b *bucket) reset() {
	b.lines = nil
	b.contentLen = 0
	b.originalDataLen = 0
}

// applyTruncation applies the shared single-line truncation behavior used by both
// direct emission and bucket flushes. Callers decide whether the current event is
// independently oversized; this helper only adds the carry-over markers and tags.
func (b *bucket) applyTruncation(msg *message.Message, content []byte, shouldTruncate bool, truncatedReason string) ([]byte, bool) {
	lastWasTruncated := b.shouldTruncate
	b.shouldTruncate = shouldTruncate

	if lastWasTruncated {
		// The previous emitted event was truncated because it was already too large on its
		// own, so this event is the continuation of that same oversized single-line log.
		content = append(message.TruncatedFlag, content...)
	}

	if b.shouldTruncate {
		// The current event is already too large on its own (or was already marked truncated
		// upstream), so we keep the single-line truncation marker on the emitted event.
		content = append(content, message.TruncatedFlag...)
		metrics.LogsTruncated.Add(1)
	}

	if lastWasTruncated || b.shouldTruncate {
		msg.ParsingExtra.IsTruncated = true
		if b.tagTruncatedLogs {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.TruncatedReasonTag(truncatedReason))
		}
	}

	return content, msg.ParsingExtra.IsTruncated
}

func (b *bucket) flush() AggregatedMessageWithTokens {
	defer b.reset()

	content := make([]byte, 0, b.contentLen)
	accumulatedRawDataLen := 0
	for i, line := range b.lines {
		if i > 0 && accumulatedRawDataLen > 0 {
			content = append(content, message.EscapedLineFeed...)
		}
		content = append(content, line.Msg.GetContent()...)
		accumulatedRawDataLen += line.Msg.RawDataLen
	}
	content = bytes.TrimSpace(content)

	msg := b.lines[0].Msg
	lineCount := len(b.lines)
	truncatedReason := "single_line"
	if lineCount > 1 {
		truncatedReason = "auto_multiline"
	}

	// Process flushes multiline buckets before an additional aggregate line can push the
	// combined content over the limit. Truncation in this path therefore comes from a
	// single oversized line or a continuation frame tracked by shouldTruncate.
	content, isTruncated := b.applyTruncation(msg, content, b.contentLen >= b.maxContentSize, truncatedReason)
	msg.SetContent(content)
	msg.RawDataLen = b.originalDataLen
	tlmTags := []string{"false", "single_line"}

	if lineCount > 1 {
		msg.ParsingExtra.IsMultiLine = true
		tlmTags[1] = "auto_multi_line"
		if b.tagMultiLineLogs {
			msg.ParsingExtra.Tags = append(msg.ParsingExtra.Tags, message.MultiLineSourceTag("auto_multiline"))
		}
	}

	if isTruncated {
		tlmTags[0] = "true"
	}

	metrics.TlmAutoMultilineAggregatorFlush.Inc(tlmTags...)
	return AggregatedMessageWithTokens{Msg: msg, Tokens: b.lines[0].Tokens}
}

func (b *bucket) emitSingle(msg *message.Message, tokens []Token) AggregatedMessageWithTokens {
	content := bytes.TrimSpace(msg.GetContent())

	// Once a bucket is exploded, each emitted event follows the normal single-line
	// truncation path. This specific line may be oversized on its own, or it may
	// already be marked truncated because it is one chunk of a split oversized line.
	content, _ = b.applyTruncation(msg, content, len(content) > b.maxContentSize || msg.ParsingExtra.IsTruncated, "single_line")
	msg.SetContent(content)
	return AggregatedMessageWithTokens{Msg: msg, Tokens: tokens}
}

func (b *bucket) explode() []AggregatedMessageWithTokens {
	defer b.reset()

	collected := make([]AggregatedMessageWithTokens, 0, len(b.lines))
	for _, line := range b.lines {
		collected = append(collected, b.emitSingle(line.Msg, line.Tokens))
	}
	return collected
}

// combiningAggregator aggregates multiline logs with a given label.
type combiningAggregator struct {
	collected          []AggregatedMessageWithTokens
	bucket             *bucket
	maxContentSize     int
	multiLineMatchInfo *status.CountInfo
	linesCombinedInfo  *status.CountInfo
}

// NewCombiningAggregator creates a new combining aggregator.
func NewCombiningAggregator(maxContentSize int, tagTruncatedLogs bool, tagMultiLineLogs bool, tailerInfo *status.InfoRegistry) Aggregator {
	multiLineMatchInfo := status.NewCountInfo("MultiLine matches")
	linesCombinedInfo := status.NewCountInfo("Lines Combined")
	tailerInfo.Register(multiLineMatchInfo)
	tailerInfo.Register(linesCombinedInfo)

	return &combiningAggregator{
		bucket: &bucket{
			tagTruncatedLogs: tagTruncatedLogs,
			tagMultiLineLogs: tagMultiLineLogs,
			maxContentSize:   maxContentSize,
			shouldTruncate:   false,
		},
		maxContentSize:     maxContentSize,
		multiLineMatchInfo: multiLineMatchInfo,
		linesCombinedInfo:  linesCombinedInfo,
	}
}

func (a *combiningAggregator) wouldOverflowBucket(msg *message.Message) bool {
	// This guard decides when to abandon aggregation and emit the buffered lines
	// individually.
	projectedLen := a.bucket.contentLen + len(msg.GetContent())
	if a.bucket.originalDataLen > 0 {
		projectedLen += len(message.EscapedLineFeed)
	}
	return projectedLen >= a.maxContentSize
}

// flushToCollected appends the flushed bucket message (if any) to a.collected.
func (a *combiningAggregator) flushToCollected() {
	if a.bucket.isEmpty() {
		a.bucket.reset()
		return
	}
	if len(a.bucket.lines) > 1 {
		a.linesCombinedInfo.Add(int64(len(a.bucket.lines) - 1))
	}
	a.collected = append(a.collected, a.bucket.flush())
}

func (a *combiningAggregator) emitSingleToCollected(msg *message.Message, tokens []Token) {
	a.collected = append(a.collected, a.bucket.emitSingle(msg, tokens))
}

func (a *combiningAggregator) explodeBucketToCollected() {
	if a.bucket.isEmpty() {
		a.bucket.reset()
		return
	}
	a.collected = append(a.collected, a.bucket.explode()...)
}

// Process processes a multiline log using a label and returns any completed messages.
func (a *combiningAggregator) Process(msg *message.Message, label Label, tokens []Token) []AggregatedMessageWithTokens {
	a.collected = a.collected[:0]

	// If `noAggregate` - flush the bucket immediately and then flush the next message.
	if label == noAggregate {
		a.flushToCollected()
		a.bucket.shouldTruncate = false // noAggregate messages should never be truncated at the beginning (Could break JSON formatted messages)
		a.bucket.add(msg, tokens)
		a.flushToCollected()
		return a.collected
	}

	// If `aggregate` and the bucket is empty - flush the next message.
	if label == aggregate && a.bucket.isEmpty() {
		a.bucket.add(msg, tokens)
		a.flushToCollected()
		return a.collected
	}

	// If `startGroup` - flush the old bucket to form a new group.
	if label == startGroup {
		a.flushToCollected()
		a.multiLineMatchInfo.Add(1)
		a.bucket.add(msg, tokens)
		if msg.RawDataLen >= a.maxContentSize {
			// A startGroup can still truncate, but only because this individual line is
			// already at the limit on its own. That's the remaining single-line truncation
			// case in this codepath; it is not caused by multiline aggregation.
			a.flushToCollected()
		}
		return a.collected
	}

	// If appending the current aggregate line would make the combined message too large,
	// emit all buffered lines as standalone messages and emit the current line on its
	// own on the normal single-line path.
	if a.wouldOverflowBucket(msg) {
		a.explodeBucketToCollected()
		a.emitSingleToCollected(msg, tokens)
		return a.collected
	}

	// We're an aggregate label within a startGroup and within the maxContentSize. Append new multiline
	a.bucket.add(msg, tokens)
	return a.collected
}

// Flush flushes the aggregator and returns any pending messages.
func (a *combiningAggregator) Flush() []AggregatedMessageWithTokens {
	a.collected = a.collected[:0]
	a.flushToCollected()
	return a.collected
}

// IsEmpty returns true if the bucket is empty.
func (a *combiningAggregator) IsEmpty() bool {
	return a.bucket.isEmpty()
}
