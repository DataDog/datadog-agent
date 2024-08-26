// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
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
	isMultiLine := len(strings.Split(content, "\\n")) > 1
	assert.Equal(t, content, string(m.GetContent()))
	assert.Equal(t, m.IsMultiLine, isMultiLine)
}

func TestNoAggregate(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second))

	ag.Aggregate(newMessage("1"), noAggregate)
	ag.Aggregate(newMessage("2"), noAggregate)
	ag.Aggregate(newMessage("3"), noAggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestNoAggregateEndsGroup(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second))

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), startGroup)
	ag.Aggregate(newMessage("3"), noAggregate) // Causes flush or last group, and flush of noAggregate message

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestAggregateGroups(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second))

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
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second))

	ag.Aggregate(newMessage("1"), aggregate)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
	assertMessageContent(t, <-outputChan, "3")
}

func TestForceFlush(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second))

	ag.Aggregate(newMessage("1"), startGroup)
	ag.Aggregate(newMessage("2"), aggregate)
	ag.Aggregate(newMessage("3"), aggregate)
	ag.Flush()

	assertMessageContent(t, <-outputChan, "1\\n2\\n3")
}

func TestAggregationTimer(t *testing.T) {

	outputChan, outputFn := makeHandler()
	ag := NewAggregator(outputFn, 100, time.Duration(1*time.Second))

	assert.Nil(t, ag.FlushChan())
	ag.Aggregate(newMessage("1"), startGroup)
	assert.NotNil(t, ag.FlushChan())

	ag.Aggregate(newMessage("2"), startGroup)
	assert.NotNil(t, ag.FlushChan())

	ag.Flush()

	assertMessageContent(t, <-outputChan, "1")
	assertMessageContent(t, <-outputChan, "2")
}
