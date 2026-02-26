// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func newMessage(content string) *message.Message {
	m := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	m.RawDataLen = len([]byte(content))
	return m
}

func assertMessageContent(t *testing.T, m *message.Message, content string) {
	t.Helper()
	isMultiLine := len(strings.Split(content, "\\n")) > 1
	assert.Equal(t, content, string(m.GetContent()))
	assert.Equal(t, m.IsMultiLine, isMultiLine)
}

func assertTrailingMultiline(t *testing.T, m *message.Message, content string) {
	t.Helper()
	assert.Equal(t, content, string(m.GetContent()))
	assert.Equal(t, m.IsMultiLine, true)
}

// NOTE: The Aggregator.Process return slice shares its backing array with the
// aggregator's internal buffer and is only valid until the next Process/Flush call.
// Tests must assert results before making the next call.

func TestNoAggregate(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	msgs := ag.Process(newMessage("1"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1")

	msgs = ag.Process(newMessage("2"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "2")

	msgs = ag.Process(newMessage("3"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "3")
}

func TestNoAggregateEndsGroup(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	require.Empty(t, ag.Process(newMessage("1"), startGroup))

	msgs := ag.Process(newMessage("2"), startGroup) // flushes "1"
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1")

	msgs = ag.Process(newMessage("3"), noAggregate) // flushes "2", then emits "3"
	require.Len(t, msgs, 2)
	assertMessageContent(t, msgs[0], "2")
	assertMessageContent(t, msgs[1], "3")
}

func TestAggregateGroups(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	// Accumulate a group
	require.Empty(t, ag.Process(newMessage("1"), startGroup))
	require.Empty(t, ag.Process(newMessage("2"), aggregate))
	require.Empty(t, ag.Process(newMessage("3"), aggregate))

	// New startGroup flushes the previous group
	msgs := ag.Process(newMessage("4"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1\\n2\\n3")

	// noAggregate flushes "4" then emits "5"
	msgs = ag.Process(newMessage("5"), noAggregate)
	require.Len(t, msgs, 2)
	assertMessageContent(t, msgs[0], "4")
	assertMessageContent(t, msgs[1], "5")
}

func TestAggregateDoesntStartGroup(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	msgs := ag.Process(newMessage("1"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1")

	msgs = ag.Process(newMessage("2"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "2")

	msgs = ag.Process(newMessage("3"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "3")
}

func TestForceFlush(t *testing.T) {
	ag := NewCombiningAggregator(100, false, false, status.NewInfoRegistry())

	require.Empty(t, ag.Process(newMessage("1"), startGroup))
	require.Empty(t, ag.Process(newMessage("2"), aggregate))
	require.Empty(t, ag.Process(newMessage("3"), aggregate))

	msgs := ag.Flush()
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "1\\n2\\n3")
}

func TestTagTruncatedLogs(t *testing.T) {
	ag := NewCombiningAggregator(10, true, false, status.NewInfoRegistry())

	// "1234567890" (len=10) as startGroup: immediately flushed (size >= maxContentSize)
	msgs := ag.Process(newMessage("1234567890"), startGroup)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "1234567890...TRUNCATED...")

	// aggregate on empty bucket: add+flush immediately; carries TRUNCATED prefix
	msgs = ag.Process(newMessage("12345678901"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "...TRUNCATED...12345678901...TRUNCATED...")

	msgs = ag.Process(newMessage("12345"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "...TRUNCATED...12345")

	// "1234" + "5678" fits (8 < 10) but adding "90" overflows
	require.Empty(t, ag.Process(newMessage("1234"), startGroup))
	require.Empty(t, ag.Process(newMessage("5678"), aggregate))

	msgs = ag.Process(newMessage("90"), aggregate)
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("auto_multiline")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "1234\\n5678...TRUNCATED...")

	assert.True(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("auto_multiline")}, msgs[1].ParsingExtra.Tags)
	assertTrailingMultiline(t, msgs[1], "...TRUNCATED...90")

	// noAggregate resets truncation carry; "00" should not be truncated
	msgs = ag.Process(newMessage("00"), noAggregate)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "00")
}

func TestSingleGroupIsTruncatedAsMultilineLog(t *testing.T) {
	ag := NewCombiningAggregator(5, true, false, status.NewInfoRegistry())

	require.Empty(t, ag.Process(newMessage("123"), startGroup))

	// "123" + "456" overflows (3+3=6 >= 5)
	msgs := ag.Process(newMessage("456"), aggregate)
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("auto_multiline")}, msgs[0].ParsingExtra.Tags)
	assertTrailingMultiline(t, msgs[0], "123...TRUNCATED...")

	assert.True(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("auto_multiline")}, msgs[1].ParsingExtra.Tags)
	assertTrailingMultiline(t, msgs[1], "...TRUNCATED...456")
}

func TestSingleLineTruncatedLogIsTaggedSingleLine(t *testing.T) {
	ag := NewCombiningAggregator(5, true, false, status.NewInfoRegistry())

	// Exactly maxContentSize — simulates truncation in the framer
	msgs := ag.Process(newMessage("12345"), startGroup)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "12345...TRUNCATED...")

	msgs = ag.Process(newMessage("456"), aggregate)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.TruncatedReasonTag("single_line")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "...TRUNCATED...456")
}

func TestTagMultiLineLogs(t *testing.T) {
	ag := NewCombiningAggregator(10, false, true, status.NewInfoRegistry())

	require.Empty(t, ag.Process(newMessage("12345"), startGroup))
	require.Empty(t, ag.Process(newMessage("6789"), aggregate))

	// "12345\n6789" (11 bytes) + "1" (1) overflows at 12 >= 10
	msgs := ag.Process(newMessage("1"), aggregate)
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].ParsingExtra.IsMultiLine)
	assert.True(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.MultiLineSourceTag("auto_multiline")}, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "12345\\n6789...TRUNCATED...")

	assert.True(t, msgs[1].ParsingExtra.IsMultiLine)
	assert.True(t, msgs[1].ParsingExtra.IsTruncated)
	assert.Equal(t, []string{message.MultiLineSourceTag("auto_multiline")}, msgs[1].ParsingExtra.Tags)
	assertTrailingMultiline(t, msgs[1], "...TRUNCATED...1")

	msgs = ag.Process(newMessage("2"), noAggregate)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].ParsingExtra.IsMultiLine)
	assert.False(t, msgs[0].ParsingExtra.IsTruncated)
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
	assertMessageContent(t, msgs[0], "2")
}

func TestSingleLineTooLongTruncation(t *testing.T) {
	ag := NewCombiningAggregator(5, false, true, status.NewInfoRegistry())

	// Phase 1: multi-line log where messages overflow
	require.Empty(t, ag.Process(newMessage("123"), startGroup))

	// "123"(3) + "456"(3) = 6 >= 5 → overflow, emits 2 messages
	msgs := ag.Process(newMessage("456"), aggregate)
	require.Len(t, msgs, 2)
	assertTrailingMultiline(t, msgs[0], "123...TRUNCATED...")
	assertTrailingMultiline(t, msgs[1], "...TRUNCATED...456")

	// bucket empty, add "123456" → immediately flushed (6 >= 5)
	msgs = ag.Process(newMessage("123456"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	// bucket empty, shouldTruncate=true, add "123" → flushed with prefix
	msgs = ag.Process(newMessage("123"), aggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123")

	// Force flush: start empty group — nothing emitted
	require.Empty(t, ag.Process(newMessage(""), startGroup))

	// Phase 2: single-line logs each too large
	msgs = ag.Process(newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	msgs = ag.Process(newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123456...TRUNCATED...")

	msgs = ag.Process(newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123456...TRUNCATED...")

	// "123" fits (3 < 5): buffered
	require.Empty(t, ag.Process(newMessage("123"), startGroup))

	// Force flush: flushes "123" with prefix
	msgs = ag.Process(newMessage(""), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123")

	// Phase 3: noAggregate clears the TRUNCATED carry
	msgs = ag.Process(newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	// noAggregate: shouldTruncate is explicitly cleared → no prefix
	msgs = ag.Process(newMessage("123456"), noAggregate)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "123456...TRUNCATED...")

	msgs = ag.Process(newMessage("123456"), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123456...TRUNCATED...")

	require.Empty(t, ag.Process(newMessage("123"), startGroup))

	msgs = ag.Process(newMessage(""), startGroup)
	require.Len(t, msgs, 1)
	assertMessageContent(t, msgs[0], "...TRUNCATED...123")
}

// Tests for RegexAggregator

func TestRegexAggregatorNoMatchSendsLinesIndividually(t *testing.T) {
	re := regexp.MustCompile(`^NEVER_MATCHES_ANYTHING$`)
	ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

	msgs := ag.Process(newMessage("first line"), noAggregate)
	require.Empty(t, msgs, "first line should be buffered until a second line arrives")

	msgs = ag.Process(newMessage("second line"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "first line", string(msgs[0].GetContent()))

	msgs = ag.Process(newMessage("third line"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "second line", string(msgs[0].GetContent()))

	msgs = ag.Flush()
	require.Len(t, msgs, 1)
	assert.Equal(t, "third line", string(msgs[0].GetContent()))
}

func TestRegexAggregatorNoMatchThenMatchSwitchesToMultiLine(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

	// Lines before the first match are sent individually
	require.Empty(t, ag.Process(newMessage("no match line 1"), noAggregate))

	msgs := ag.Process(newMessage("no match line 2"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "no match line 1", string(msgs[0].GetContent()))

	// Pattern matches — flushes buffered line, starts multiline aggregation
	msgs = ag.Process(newMessage("START of multiline"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "no match line 2", string(msgs[0].GetContent()))

	// Continuation is now aggregated (pattern has matched)
	require.Empty(t, ag.Process(newMessage("continuation line"), noAggregate))

	// Next match flushes the combined group
	msgs = ag.Process(newMessage("START of second group"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "START of multiline\\ncontinuation line", string(msgs[0].GetContent()))

	msgs = ag.Flush()
	require.Len(t, msgs, 1)
	assert.Equal(t, "START of second group", string(msgs[0].GetContent()))
}

func TestRegexAggregatorFirstLineMatchesWorksNormally(t *testing.T) {
	re := regexp.MustCompile(`^START`)
	ag := NewRegexAggregator(re, 1000, false, status.NewInfoRegistry(), "multi_line")

	require.Empty(t, ag.Process(newMessage("START first group"), noAggregate))
	require.Empty(t, ag.Process(newMessage("continuation"), noAggregate))

	msgs := ag.Process(newMessage("START second group"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "START first group\\ncontinuation", string(msgs[0].GetContent()))

	msgs = ag.Flush()
	require.Len(t, msgs, 1)
	assert.Equal(t, "START second group", string(msgs[0].GetContent()))
}

// Tests for detectingAggregator

func TestDetectingAggregator_TagsMultilineStartOnly(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry())

	// startGroup: stored as pending, nothing emitted
	require.Empty(t, ag.Process(newMessage("Error: Exception"), startGroup))

	// First aggregate: emits tagged startGroup + current line
	msgs := ag.Process(newMessage("  at line 1"), aggregate)
	require.Len(t, msgs, 2)
	assert.Equal(t, "Error: Exception", string(msgs[0].GetContent()))
	assert.Contains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
	assert.Equal(t, "  at line 1", string(msgs[1].GetContent()))
	assert.NotContains(t, msgs[1].ParsingExtra.Tags, "auto_multiline_detected:true")

	// Subsequent aggregate: emitted immediately without tags
	msgs = ag.Process(newMessage("  at line 2"), aggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "  at line 2", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_SingleLineNotTagged(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry())

	// startGroup: stored
	require.Empty(t, ag.Process(newMessage("Single line 1"), startGroup))

	// Another startGroup flushes the previous without tagging
	msgs := ag.Process(newMessage("Single line 2"), startGroup)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Single line 1", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")

	// Flush to get the second message
	msgs = ag.Flush()
	require.Len(t, msgs, 1)
	assert.Equal(t, "Single line 2", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_NoAggregateOutputsImmediately(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry())

	msgs := ag.Process(newMessage("No aggregate 1"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "No aggregate 1", string(msgs[0].GetContent()))
	assert.Empty(t, msgs[0].ParsingExtra.Tags)

	msgs = ag.Process(newMessage("No aggregate 2"), noAggregate)
	require.Len(t, msgs, 1)
	assert.Equal(t, "No aggregate 2", string(msgs[0].GetContent()))
	assert.Empty(t, msgs[0].ParsingExtra.Tags)
}

func TestDetectingAggregator_FlushPendingMessage(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry())

	require.Empty(t, ag.Process(newMessage("Pending message"), startGroup))

	msgs := ag.Flush()
	require.Len(t, msgs, 1)
	assert.Equal(t, "Pending message", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_MixedSingleAndMultiLine(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry())

	// Single line stored
	require.Empty(t, ag.Process(newMessage("Single"), startGroup))

	// New startGroup flushes "Single" without tag
	msgs := ag.Process(newMessage("Multi start"), startGroup)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Single", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")

	// aggregate: tags "Multi start" and emits + continuation
	msgs = ag.Process(newMessage("  continuation"), aggregate)
	require.Len(t, msgs, 2)
	assert.Equal(t, "Multi start", string(msgs[0].GetContent()))
	assert.Contains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
	assert.Equal(t, "  continuation", string(msgs[1].GetContent()))
	assert.NotContains(t, msgs[1].ParsingExtra.Tags, "auto_multiline_detected:true")

	// Another single line stored
	require.Empty(t, ag.Process(newMessage("Another single"), startGroup))

	msgs = ag.Flush()
	require.Len(t, msgs, 1)
	assert.Equal(t, "Another single", string(msgs[0].GetContent()))
	assert.NotContains(t, msgs[0].ParsingExtra.Tags, "auto_multiline_detected:true")
}

func TestDetectingAggregator_IsEmpty(t *testing.T) {
	ag := NewDetectingAggregator(status.NewInfoRegistry())

	assert.True(t, ag.IsEmpty())

	require.Empty(t, ag.Process(newMessage("Pending"), startGroup))
	assert.False(t, ag.IsEmpty())

	msgs := ag.Flush()
	require.Len(t, msgs, 1)
	assert.True(t, ag.IsEmpty())

	msgs = ag.Process(newMessage("Immediate"), noAggregate)
	require.Len(t, msgs, 1)
	assert.True(t, ag.IsEmpty())
}
