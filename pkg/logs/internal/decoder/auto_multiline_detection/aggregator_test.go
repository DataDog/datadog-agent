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
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), noAggregate)
	ag.Aggregate(newMessage("2"), noAggregate)
	ag.Aggregate(newMessage("3"), noAggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestNoAggregateEndsGroup(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), startGroup)
	ag.Aggregate(newMessage("3"), noAggregate) // Causes flush or last group, and flush of noAggregate message

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestAggregateGroups(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

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
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), aggregate)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestForceFlush(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)
	ag.Flush()

	assertMessageContent(t, <-outputChan, "1\\n2\\n3")
}

func TestTagTruncatedLogs(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 10, true, false, status.NewInfoRegistry())

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
	ag := NewAggregator(outputFn, 5, true, false, status.NewInfoRegistry())

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
	ag := NewAggregator(outputFn, 5, true, false, status.NewInfoRegistry())

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
	ag := NewAggregator(outputFn, 10, false, true, status.NewInfoRegistry())

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
	ag := NewAggregator(outputFn, 5, false, true, status.NewInfoRegistry())

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
