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
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), false)

	ag.Aggregate(newMessage("1"), noAggregate)
	ag.Aggregate(newMessage("2"), noAggregate)
	ag.Aggregate(newMessage("3"), noAggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestNoAggregateEndsGroup(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), false)

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), startGroup)
	ag.Aggregate(newMessage("3"), noAggregate) // Causes flush or last group, and flush of noAggregate message

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestAggregateGroups(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), false)

	// Aggregated log
	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)

	// Aggregated log
	ag.Aggregate(newMessage("4"), startGroup)

	// Aggregated log
	ag.Aggregate(newMessage("5"), noAggregate)

	assertMessageContent(t, <-outputChan, "1\\n2\\n3")
	assertMessageContent(t, <-outputChan, "4")
	assertMessageContent(t, <-outputChan, "5")
}

func TestAggregateDoesntStartGroup(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), false)

	ag.Aggregate(newMessage("1"), aggregate)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestForceFlush(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), false)

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)
	ag.Flush()

	assertMessageContent(t, <-outputChan, "1\\n2\\n3")
}

func TestTagTruncatedLogs(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 10, true, false, status.NewInfoRegistry(), false)

	// First 3 should be tagged as single line logs since they are too big to aggregate no matter what the label is.
	ag.Aggregate(newMessage("1234567890"), startGroup)
	ag.Aggregate(newMessage("12345678901"), aggregate)
	ag.Aggregate(newMessage("12345"), aggregate)

	// Next 3 lines should be tagged as multiline since they were truncated after a group was started
	ag.Aggregate(newMessage("1234"), startGroup)
	ag.Aggregate(newMessage("5678"), aggregate)
	ag.Aggregate(newMessage("90"), aggregate)

	// No aggregate should not be truncated
	ag.Aggregate(newMessage("00"), noAggregate)

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
	ag := NewAggregator(outputFn, 5, true, false, status.NewInfoRegistry(), false)

	ag.Aggregate(newMessage("123"), startGroup)
	ag.Aggregate(newMessage("456"), aggregate)

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
	ag := NewAggregator(outputFn, 5, true, false, status.NewInfoRegistry(), false)

	ag.Aggregate(newMessage("12345"), startGroup) // Exactly the size of the max message size - simulates truncation in the framer
	ag.Aggregate(newMessage("456"), aggregate)

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
	ag := NewAggregator(outputFn, 10, false, true, status.NewInfoRegistry(), false)

	ag.Aggregate(newMessage("12345"), startGroup)
	ag.Aggregate(newMessage("6789"), aggregate)
	ag.Aggregate(newMessage("1"), aggregate) // Causes overflow, truncate and flush
	ag.Aggregate(newMessage("2"), noAggregate)

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
	ag := NewAggregator(outputFn, 5, false, true, status.NewInfoRegistry(), false)

	// Multi line log where each message is too large except the last one
	ag.Aggregate(newMessage("123"), startGroup)
	ag.Aggregate(newMessage("456"), aggregate)
	ag.Aggregate(newMessage("123456"), aggregate)
	ag.Aggregate(newMessage("123"), aggregate)
	// Force a flush
	ag.Aggregate(newMessage(""), startGroup)

	msg := <-outputChan
	assertTrailingMultiline(t, msg, "123...TRUNCATED...")
	msg = <-outputChan
	assertTrailingMultiline(t, msg, "...TRUNCATED...456")
	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123")

	// Single line logs where each message is too large except the last
	ag.Aggregate(newMessage("123456"), startGroup)
	ag.Aggregate(newMessage("123456"), startGroup)
	ag.Aggregate(newMessage("123456"), startGroup)
	ag.Aggregate(newMessage("123"), startGroup)
	// Force a flush
	ag.Aggregate(newMessage(""), startGroup)

	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123")

	// No aggregate logs should never be truncated from the previous message (Could break a JSON payload)
	ag.Aggregate(newMessage("123456"), startGroup)
	ag.Aggregate(newMessage("123456"), noAggregate)
	ag.Aggregate(newMessage("123456"), startGroup)
	ag.Aggregate(newMessage("123"), startGroup)
	// Force a flush
	ag.Aggregate(newMessage(""), startGroup)

	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123456...TRUNCATED...")
	msg = <-outputChan
	assertMessageContent(t, msg, "...TRUNCATED...123")
}

// Detection-only mode tests (using tagOnlyBucket)

func TestDetectionOnly_SingleLineNotTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), true)

	// Single-line logs should not be tagged
	ag.Aggregate(newMessage("single line 1"), startGroup)
	ag.Aggregate(newMessage("single line 2"), startGroup)
	ag.Flush()

	msg1 := <-outputChan
	assert.Equal(t, "single line 1", string(msg1.GetContent()))
	assert.Empty(t, msg1.ParsingExtra.Tags, "Single-line log should not have processing tags")

	msg2 := <-outputChan
	assert.Equal(t, "single line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags, "Single-line log should not have processing tags")
}

func TestDetectionOnly_TwoLineGroupTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), true)

	// 2-line multiline group
	ag.Aggregate(newMessage("line 1"), startGroup)
	ag.Aggregate(newMessage("line 2"), aggregate)
	ag.Flush()

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Contains(t, msg1.ParsingExtra.Tags, "auto_multiline_group_size:2")

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Contains(t, msg2.ParsingExtra.Tags, "auto_multiline_group_size:2")
}

func TestDetectionOnly_ThreeLineGroupTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), true)

	// 3-line multiline group
	ag.Aggregate(newMessage("line 1"), startGroup)
	ag.Aggregate(newMessage("line 2"), aggregate)
	ag.Aggregate(newMessage("line 3"), aggregate)
	ag.Flush()

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Contains(t, msg1.ParsingExtra.Tags, "auto_multiline_group_size:3")

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Contains(t, msg2.ParsingExtra.Tags, "auto_multiline_group_size:3")

	msg3 := <-outputChan
	assert.Equal(t, "line 3", string(msg3.GetContent()))
	assert.Contains(t, msg3.ParsingExtra.Tags, "auto_multiline_group_size:3")
}

func TestDetectionOnly_MultipleGroups(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), true)

	// First group: 2 lines
	ag.Aggregate(newMessage("group1 line1"), startGroup)
	ag.Aggregate(newMessage("group1 line2"), aggregate)

	// Second group: 3 lines
	ag.Aggregate(newMessage("group2 line1"), startGroup)
	ag.Aggregate(newMessage("group2 line2"), aggregate)
	ag.Aggregate(newMessage("group2 line3"), aggregate)

	// Single line
	ag.Aggregate(newMessage("single line"), startGroup)
	ag.Flush()

	// First group messages
	msg := <-outputChan
	assert.Equal(t, "group1 line1", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_group_size:2")

	msg = <-outputChan
	assert.Equal(t, "group1 line2", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_group_size:2")

	// Second group messages
	msg = <-outputChan
	assert.Equal(t, "group2 line1", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_group_size:3")

	msg = <-outputChan
	assert.Equal(t, "group2 line2", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_group_size:3")

	msg = <-outputChan
	assert.Equal(t, "group2 line3", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_group_size:3")

	// Single line message
	msg = <-outputChan
	assert.Equal(t, "single line", string(msg.GetContent()))
	assert.Empty(t, msg.ParsingExtra.Tags)
}

func TestDetectionOnly_NoAggregateNotTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), true)

	// NoAggregate messages should never be tagged
	ag.Aggregate(newMessage("line 1"), noAggregate)
	ag.Aggregate(newMessage("line 2"), noAggregate)

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Empty(t, msg1.ParsingExtra.Tags)

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags)
}

func TestDetectionOnly_AggregateWithoutStartGroup(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), true)

	// Aggregate labels without a startGroup should be flushed immediately and not tagged
	ag.Aggregate(newMessage("line 1"), aggregate)
	ag.Aggregate(newMessage("line 2"), aggregate)

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Empty(t, msg1.ParsingExtra.Tags)

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags)
}

func TestDetectionOnly_ContentNotCombined(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry(), true)

	// In detection-only mode, content should NOT be combined
	ag.Aggregate(newMessage("line 1"), startGroup)
	ag.Aggregate(newMessage("line 2"), aggregate)
	ag.Aggregate(newMessage("line 3"), aggregate)
	ag.Flush()

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.NotContains(t, string(msg1.GetContent()), "\\n", "Content should not be combined")

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.NotContains(t, string(msg2.GetContent()), "\\n", "Content should not be combined")

	msg3 := <-outputChan
	assert.Equal(t, "line 3", string(msg3.GetContent()))
	assert.NotContains(t, string(msg3.GetContent()), "\\n", "Content should not be combined")
}
