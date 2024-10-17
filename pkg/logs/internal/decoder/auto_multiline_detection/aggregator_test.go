// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"
	"time"

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
	return message.NewRawMessage([]byte(content), message.StatusInfo, len([]byte(content)), "")
}

func assertMessageContent(t *testing.T, m *message.Message, content string) {
	assert.Equal(t, content, string(m.GetContent()))
}

func TestNoAggregate(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second), false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), noAggregate)
	ag.Aggregate(newMessage("2"), noAggregate)
	ag.Aggregate(newMessage("3"), noAggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestNoAggregateEndsGroup(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second), false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), startGroup)
	ag.Aggregate(newMessage("3"), noAggregate) // Causes flush or last group, and flush of noAggregate message

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestAggregateGroups(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second), false, false, status.NewInfoRegistry())

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
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second), false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), aggregate)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestForceFlush(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second), false, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)
	ag.Flush()

	assertMessageContent(t, <-outputChan, "1\\n2\\n3")
}

func TestAggregationTimer(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second), false, false, status.NewInfoRegistry())

	assert.Nil(t, ag.FlushChan())
	ag.Aggregate(newMessage("1"), startGroup)
	assert.NotNil(t, ag.FlushChan())

	ag.Aggregate(newMessage("2"), startGroup)
	assert.NotNil(t, ag.FlushChan())

	ag.Flush()

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
}

func TestTagTruncatedLogs(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 10, time.Duration(1*time.Second), true, false, status.NewInfoRegistry())

	ag.Aggregate(newMessage("1234567890"), startGroup)
	ag.Aggregate(newMessage("123456789"), startGroup)
	ag.Aggregate(newMessage("123456789"), startGroup)
	ag.Aggregate(newMessage("1234567890"), aggregate) // Causes overflow, truncate and flush
	ag.Aggregate(newMessage("12345"), aggregate)
	ag.Aggregate(newMessage("6789"), aggregate)
	ag.Aggregate(newMessage("3"), noAggregate)
	ag.Aggregate(newMessage("1234567890"), noAggregate)

	msg := <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	// First log line should be considered a single line log since it was not followed by a multi-line log.
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
	assertMessageContent(t, msg, "1234567890...TRUNCATED...")

	msg = <-outputChan
	assert.False(t, msg.ParsingExtra.IsTruncated)
	// Second should not be truncated.
	assert.Empty(t, msg.ParsingExtra.Tags)
	assertMessageContent(t, msg, "123456789")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	// Third log shhould be considered a multi-line log since it was followed by another log to be aggregated with.
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertMessageContent(t, msg, "123456789...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertMessageContent(t, msg, "...TRUNCATED...1234567890...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertMessageContent(t, msg, "...TRUNCATED...12345...TRUNCATED...")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("auto_multiline")})
	assertMessageContent(t, msg, "...TRUNCATED...6789")

	msg = <-outputChan
	assert.False(t, msg.ParsingExtra.IsTruncated)
	assert.Empty(t, msg.ParsingExtra.Tags)
	assertMessageContent(t, msg, "3")

	msg = <-outputChan
	assert.True(t, msg.ParsingExtra.IsTruncated)
	// Last log line should be considered a single line log since it was no_aggregate
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
	assertMessageContent(t, msg, "1234567890...TRUNCATED...")
}

func TestTagMultiLineLogs(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 10, time.Duration(1*time.Second), false, true, status.NewInfoRegistry())

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
	assertMessageContent(t, msg, "...TRUNCATED...1")

	msg = <-outputChan
	assert.False(t, msg.ParsingExtra.IsMultiLine)
	assert.False(t, msg.ParsingExtra.IsTruncated)
	assert.Empty(t, msg.ParsingExtra.Tags)
	assertMessageContent(t, msg, "2")
}

func TestStartGruopIsNotTruncatedWithoutAggreagation(t *testing.T) {
	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 5, time.Duration(1*time.Second), true, true, status.NewInfoRegistry())

	ag.Aggregate(newMessage("123456"), startGroup)
	// Force a flush
	ag.Aggregate(newMessage(""), startGroup)

	msg := <-outputChan
	assertMessageContent(t, msg, "123456...TRUNCATED...")
	assert.Equal(t, msg.ParsingExtra.Tags, []string{message.TruncatedReasonTag("single_line")})
}
