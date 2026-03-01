// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func makeHandler() (chan *message.Message, func(*message.Message)) {
	ch := make(chan *message.Message, 20)
	return ch, func(m *message.Message) {
		ch <- m
	}
}

func newMessage(content string) *message.Message {

	m := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	m.RawDataLen = len([]byte(content))
	return m
}

func assertMessageContent(t *testing.T, m *message.Message, content string) {
	isMultiLine := len(strings.Split(content, "\\n")) > 1
	assert.Equal(t, content, string(m.GetContent()))
	assert.Equal(t, m.IsMultiLine, isMultiLine)
}

func assertTrailingMultiline(t *testing.T, m *message.Message, content string) {
	assert.Equal(t, content, string(m.GetContent()))
	assert.Equal(t, m.IsMultiLine, true)
}

func TestNoAggregate(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Process(newMessage("1"), noAggregate)
	ag.Process(newMessage("2"), noAggregate)
	ag.Process(newMessage("3"), noAggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestNoAggregateEndsGroup(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Process(newMessage("1"), startGroup)
	ag.Process(newMessage("2"), startGroup)
	ag.Process(newMessage("3"), noAggregate) // Causes flush or last group, and flush of noAggregate message

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestAggregateGroups(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	// Aggregated log
	ag.Process(newMessage("1"), startGroup)
	ag.Process(newMessage("2"), aggregate)
	ag.Process(newMessage("3"), aggregate)

	// Aggregated log
	ag.Process(newMessage("4"), startGroup)

	// Aggregated log
	ag.Process(newMessage("5"), noAggregate)

	assertMessageContent(t, <-outputChan, "1\\n2\\n3")
	assertMessageContent(t, <-outputChan, "4")
	assertMessageContent(t, <-outputChan, "5")
}

func TestAggregateDoesntStartGroup(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Process(newMessage("1"), aggregate)
	ag.Process(newMessage("2"), aggregate)
	ag.Process(newMessage("3"), aggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestForceFlush(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Process(newMessage("1"), startGroup)
	ag.Process(newMessage("2"), aggregate)
	ag.Process(newMessage("3"), aggregate)
	ag.Flush()

	assertMessageContent(t, <-outputChan, "1\\n2\\n3")
}

func TestTagTruncatedLogs(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 10, true, false, status.NewInfoRegistry())

	// First 3 should be tagged as single line logs since they are too big to aggregate no matter what the label is.
	ag.Process(newMessage("1234567890"), startGroup)
	ag.Process(newMessage("12345678901"), aggregate)
	ag.Process(newMessage("12345"), aggregate)

	// Next 3 lines should be tagged as multiline since they were truncated after a group was started
	ag.Process(newMessage("1234"), startGroup)
	ag.Process(newMessage("5678"), aggregate)
	ag.Process(newMessage("90"), aggregate)

	// No aggregate should not be truncated
	ag.Process(newMessage("00"), noAggregate)

	msg := <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
	assertMessageContent(t, msg, "1234567890...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
	assertMessageContent(t, msg, "...TRUNCATED...12345678901...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
	assertMessageContent(t, msg, "...TRUNCATED...12345")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertMessageContent(t, msg, "1234\\n5678...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertTrailingMultiline(t, msg, "...TRUNCATED...90")

	msg = <-outputChan
	assert.False(t, msg.ParsingExtra.IsTruncated)
	assert.Empty(t, msg.ParsingExtra.Tags)
	assertMessageContent(t, msg, "00")
}

func TestSingleGroupIsTruncatedAsMultilineLog(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 5, true, false, status.NewInfoRegistry())

	ag.Process(newMessage("123"), startGroup)
	ag.Process(newMessage("456"), aggregate)

	msg := <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertTrailingMultiline(t, msg, "123...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertTrailingMultiline(t, msg, "...TRUNCATED...456")
}

func TestSingleLineTruncatedLogIsTaggedSingleLine(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 5, true, false, status.NewInfoRegistry())

	ag.Process(newMessage("12345"), startGroup) // Exactly the size of the max message size - simulates truncation in the framer
	ag.Process(newMessage("456"), aggregate)

	msg := <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
	assertMessageContent(t, msg, "12345...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
	assertMessageContent(t, msg, "...TRUNCATED...456")
}

func TestTagMultiLineLogs(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 10, false, true, status.NewInfoRegistry())

	ag.Process(newMessage("12345"), startGroup)
	ag.Process(newMessage("6789"), aggregate)
	ag.Process(newMessage("1"), aggregate) // Causes overflow, truncate and flush
	ag.Process(newMessage("2"), noAggregate)

	msg := <-outputChan
	assert.True(t, msg.ParsingExtra.IsMultiLine)
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.MultiLineSourceTag("auto_multiline")})
	assertMessageContent(t, msg, "12345\\n6789...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsMultiLine)
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.MultiLineSourceTag("auto_multiline")})
	assertTrailingMultiline(t, msg, "...TRUNCATED...1")

	msg = <-outputChan
	assert.False(t, msg.ParsingExtra.IsMultiLine)
	assert.False(t, msg.ParsingExtra.IsTruncated)
	assert.Empty(t, msg.ParsingExtra.Tags)
	assertMessageContent(t, msg, "2")
}

func TestSingleLineTooLongTruncation(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewCombiningAggregator(outputFn, 5, false, true, status.NewInfoRegistry())

	// Multi line log where each message is too large except the last one
	ag.Process(newMessage("123"), startGroup)
	ag.Process(newMessage("456"), aggregate)
	ag.Process(newMessage("123456"), aggregate)
	ag.Process(newMessage("123"), aggregate)
	// Force a flush
	ag.Process(newMessage(""), startGroup)

	msg := <-outputChan
	assertTrailingMultiline(t, msg, "123...TRUNCATED...")
	msg = <-outputChan
	assertTrailingMultiline(t, msg, "...TRUNCATED...456")
	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123")

	// Single line logs where each message is too large except the last
	ag.Process(newMessage("123456"), startGroup)
	ag.Process(newMessage("123456"), startGroup)
	ag.Process(newMessage("123456"), startGroup)
	ag.Process(newMessage("123"), startGroup)
	// Force a flush
	ag.Process(newMessage(""), startGroup)

	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123")

	// No aggregate logs should never be truncated from the previous message (Could break a JSON payload)
	ag.Process(newMessage("123456"), startGroup)
	ag.Process(newMessage("123456"), noAggregate)
	ag.Process(newMessage("123456"), startGroup)
	ag.Process(newMessage("123"), startGroup)
	// Force a flush
	ag.Process(newMessage(""), startGroup)

	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123")
}

// Tests for detectingAggregator

func TestDetectingAggregator_TagsMultilineStartOnly(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 100, false)

	// Multiline log: startGroup followed by aggregate
	ag.Process(newMessage("Error: Exception"), startGroup)
	ag.Process(newMessage("  at line 1"), aggregate)
	ag.Process(newMessage("  at line 2"), aggregate)

	// The startGroup message should be tagged
	msg := <-outputChan
	assert.Equal(t, "Error: Exception", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	// Aggregate messages should be output immediately without tags
	msg = <-outputChan
	assert.Equal(t, "  at line 1", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	msg = <-outputChan
	assert.Equal(t, "  at line 2", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_SingleLineNotTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 100, false)

	// startGroup followed by another startGroup (no aggregate) means first was single-line
	ag.Process(newMessage("Single line 1"), startGroup)
	ag.Process(newMessage("Single line 2"), startGroup)

	// First message should NOT be tagged since no aggregate followed
	msg := <-outputChan
	assert.Equal(t, "Single line 1", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	// Flush to get the second message
	ag.Flush()
	msg = <-outputChan
	assert.Equal(t, "Single line 2", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_NoAggregateOutputsImmediately(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 100, false)

	ag.Process(newMessage("No aggregate 1"), noAggregate)
	ag.Process(newMessage("No aggregate 2"), noAggregate)

	msg := <-outputChan
	assert.Equal(t, "No aggregate 1", string(msg.GetContent()))
	assert.Empty(t, msg.ParsingExtra.Tags)

	msg = <-outputChan
	assert.Equal(t, "No aggregate 2", string(msg.GetContent()))
	assert.Empty(t, msg.ParsingExtra.Tags)
}

func TestDetectingAggregator_FlushPendingMessage(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 100, false)

	// startGroup but no following aggregate
	ag.Process(newMessage("Pending message"), startGroup)

	// Nothing should be output yet
	select {
	case <-outputChan:
		t.Fatal("Expected no output before flush")
	default:
	}

	// Flush should output the pending message without tags
	ag.Flush()
	msg := <-outputChan
	assert.Equal(t, "Pending message", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_MixedSingleAndMultiLine(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 100, false)

	// Single line
	ag.Process(newMessage("Single"), startGroup)
	// Multiline starts
	ag.Process(newMessage("Multi start"), startGroup)
	ag.Process(newMessage("  continuation"), aggregate)
	// Another single line
	ag.Process(newMessage("Another single"), startGroup)

	// First message (single line, not tagged)
	msg := <-outputChan
	assert.Equal(t, "Single", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	// Second message (multiline start, tagged)
	msg = <-outputChan
	assert.Equal(t, "Multi start", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	// Third message (continuation, not tagged)
	msg = <-outputChan
	assert.Equal(t, "  continuation", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	// Flush for the last message
	ag.Flush()
	msg = <-outputChan
	assert.Equal(t, "Another single", string(msg.GetContent()))
	assert.NotContains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_IsEmpty(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 100, false)

	// Should be empty initially
	assert.True(t, ag.IsEmpty())

	// Add a startGroup (pending message)
	ag.Process(newMessage("Pending"), startGroup)
	assert.False(t, ag.IsEmpty())

	// Flush it
	ag.Flush()
	<-outputChan
	assert.True(t, ag.IsEmpty())

	// Add and immediately output with noAggregate
	ag.Process(newMessage("Immediate"), noAggregate)
	<-outputChan
	assert.True(t, ag.IsEmpty())
}

// COAT telemetry tests

func TestDetectingAggregator_COATTelemetry_WouldCombine(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 1000, true)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	// startGroup followed by two aggregates: both aggregates would be combined
	ag.Process(newMessage("timestamp line"), startGroup)
	ag.Process(newMessage("  continuation 1"), aggregate)
	ag.Process(newMessage("  continuation 2"), aggregate)

	// Drain output
	<-outputChan // startGroup tagged and emitted when first aggregate arrives
	<-outputChan // first aggregate emitted
	<-outputChan // second aggregate emitted

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	assert.Equal(t, float64(3), totalAfter-totalBefore)
	assert.Equal(t, float64(2), combineAfter-combineBefore)
}

func TestDetectingAggregator_COATTelemetry_NoCombineForStandaloneAggregates(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 1000, true)

	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	// Aggregate lines without a preceding startGroup should NOT count as would-combine
	ag.Process(newMessage("orphan 1"), aggregate)
	ag.Process(newMessage("orphan 2"), aggregate)

	<-outputChan
	<-outputChan

	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	assert.Equal(t, float64(0), combineAfter-combineBefore)
}

func TestDetectingAggregator_COATTelemetry_WouldTruncate(t *testing.T) {
	outputChan, outputFn := makeHandler()
	// maxContentSize=20 so combining will overflow
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 20, true)

	truncBefore := metrics.TlmAutoMultilineWouldTruncateLines.WithValues().Get()
	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()

	// startGroup(10 bytes content) + aggregate(15 bytes content) → 15 + 10 = 25 >= 20 → truncation
	ag.Process(newMessage("1234567890"), startGroup)     // 10 bytes content
	ag.Process(newMessage("123456789012345"), aggregate) // 15 bytes, RawDataLen=15, 15+10>=20 → truncate

	<-outputChan // startGroup tagged
	<-outputChan // aggregate emitted

	truncAfter := metrics.TlmAutoMultilineWouldTruncateLines.WithValues().Get()
	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()

	// Both lines (startGroup + overflowing aggregate) belong to the truncated group
	assert.Equal(t, float64(2), truncAfter-truncBefore)
	assert.Equal(t, float64(2), totalAfter-totalBefore)
}

func TestDetectingAggregator_COATTelemetry_NoTruncateForOversizedSingleLine(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 5, true)

	truncBefore := metrics.TlmAutoMultilineWouldTruncateLines.WithValues().Get()

	// A single startGroup >= maxContentSize is excluded from truncation counts
	// (it would be truncated regardless of auto-multiline)
	ag.Process(newMessage("12345"), startGroup) // RawDataLen=5 >= maxContentSize=5
	ag.Process(newMessage("67"), aggregate)     // Not in group since startGroup was oversized

	<-outputChan
	<-outputChan

	truncAfter := metrics.TlmAutoMultilineWouldTruncateLines.WithValues().Get()
	assert.Equal(t, float64(0), truncAfter-truncBefore)
}

func TestDetectingAggregator_COATTelemetry_NoCountsWhenNotDefaultPath(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 1000, false)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	ag.Process(newMessage("timestamp line"), startGroup)
	ag.Process(newMessage("  continuation"), aggregate)

	<-outputChan
	<-outputChan

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()

	assert.Equal(t, float64(0), totalAfter-totalBefore)
	assert.Equal(t, float64(0), combineAfter-combineBefore)
}

func TestDetectingAggregator_COATTelemetry_MultiGroupWithTruncation(t *testing.T) {
	outputChan, outputFn := makeHandler()
	// maxContentSize=15 to make the second group truncate
	ag := NewDetectingAggregator(outputFn, status.NewInfoRegistry(), 15, true)

	totalBefore := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineBefore := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncBefore := metrics.TlmAutoMultilineWouldTruncateLines.WithValues().Get()

	// Group 1: fits (5 content + 2 LF + 3 content = 10 < 15)
	ag.Process(newMessage("12345"), startGroup) // 5 bytes
	ag.Process(newMessage("678"), aggregate)    // RawDataLen=3, 3+5=8 < 15 → ok, bufLen = 5+2+3 = 10

	// Group 2: will truncate (10 content + RawDataLen=10 → 10+10=20 >= 15)
	ag.Process(newMessage("1234567890"), startGroup) // 10 bytes, starts new group
	ag.Process(newMessage("1234567890"), aggregate)  // RawDataLen=10, 10+10>=15 → truncate

	// noAggregate standalone
	ag.Process(newMessage("standalone"), noAggregate)

	// Drain all output (5 messages: group1 start tagged, group1 agg, group2 start, group2 agg, standalone)
	for i := 0; i < 5; i++ {
		<-outputChan
	}

	totalAfter := metrics.TlmAutoMultilineTotalLines.WithValues().Get()
	combineAfter := metrics.TlmAutoMultilineWouldCombine.WithValues().Get()
	truncAfter := metrics.TlmAutoMultilineWouldTruncateLines.WithValues().Get()

	assert.Equal(t, float64(5), totalAfter-totalBefore)
	assert.Equal(t, float64(2), combineAfter-combineBefore) // 2 aggregate lines that would combine
	assert.Equal(t, float64(2), truncAfter-truncBefore)     // group 2 (2 lines) would truncate
}
