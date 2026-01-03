// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func TestDetector_SingleLineNotTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	detector := NewDetector(outputFn, status.NewInfoRegistry())

	// Single-line logs should not be tagged
	detector.Process(newMessage("single line 1"), startGroup)
	detector.Process(newMessage("single line 2"), startGroup)
	detector.Flush()

	msg1 := <-outputChan
	assert.Equal(t, "single line 1", string(msg1.GetContent()))
	assert.Empty(t, msg1.ParsingExtra.Tags, "Single-line log should not have processing tags")

	msg2 := <-outputChan
	assert.Equal(t, "single line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags, "Single-line log should not have processing tags")
}

func TestDetector_TwoLineGroupTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	detector := NewDetector(outputFn, status.NewInfoRegistry())

	// 2-line multiline group - only first line gets tagged
	detector.Process(newMessage("line 1"), startGroup)
	detector.Process(newMessage("line 2"), aggregate)
	detector.Flush()

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Contains(t, msg1.ParsingExtra.Tags, "auto_multiline_detected:true")

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags, "Continuation line should not have tags")
}

func TestDetector_ThreeLineGroupTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	detector := NewDetector(outputFn, status.NewInfoRegistry())

	// 3-line multiline group - only first line gets tagged
	detector.Process(newMessage("line 1"), startGroup)
	detector.Process(newMessage("line 2"), aggregate)
	detector.Process(newMessage("line 3"), aggregate)
	detector.Flush()

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Contains(t, msg1.ParsingExtra.Tags, "auto_multiline_detected:true")

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags, "Continuation line should not have tags")

	msg3 := <-outputChan
	assert.Equal(t, "line 3", string(msg3.GetContent()))
	assert.Empty(t, msg3.ParsingExtra.Tags, "Continuation line should not have tags")
}

func TestDetector_MultipleGroups(t *testing.T) {
	outputChan, outputFn := makeHandler()
	detector := NewDetector(outputFn, status.NewInfoRegistry())

	// First group: 2 lines
	detector.Process(newMessage("group1 line1"), startGroup)
	detector.Process(newMessage("group1 line2"), aggregate)

	// Second group: 3 lines
	detector.Process(newMessage("group2 line1"), startGroup)
	detector.Process(newMessage("group2 line2"), aggregate)
	detector.Process(newMessage("group2 line3"), aggregate)

	// Single line
	detector.Process(newMessage("single line"), startGroup)
	detector.Flush()

	// First group messages - only first line tagged
	msg := <-outputChan
	assert.Equal(t, "group1 line1", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	msg = <-outputChan
	assert.Equal(t, "group1 line2", string(msg.GetContent()))
	assert.Empty(t, msg.ParsingExtra.Tags)

	// Second group messages - only first line tagged
	msg = <-outputChan
	assert.Equal(t, "group2 line1", string(msg.GetContent()))
	assert.Contains(t, msg.ParsingExtra.Tags, "auto_multiline_detected:true")

	msg = <-outputChan
	assert.Equal(t, "group2 line2", string(msg.GetContent()))
	assert.Empty(t, msg.ParsingExtra.Tags)

	msg = <-outputChan
	assert.Equal(t, "group2 line3", string(msg.GetContent()))
	assert.Empty(t, msg.ParsingExtra.Tags)

	// Single line message - not tagged (no aggregate followed)
	msg = <-outputChan
	assert.Equal(t, "single line", string(msg.GetContent()))
	assert.Empty(t, msg.ParsingExtra.Tags)
}

func TestDetector_NoAggregateNotTagged(t *testing.T) {
	outputChan, outputFn := makeHandler()
	detector := NewDetector(outputFn, status.NewInfoRegistry())

	// NoAggregate messages should never be tagged
	detector.Process(newMessage("line 1"), noAggregate)
	detector.Process(newMessage("line 2"), noAggregate)

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Empty(t, msg1.ParsingExtra.Tags)

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags)
}

func TestDetector_AggregateWithoutStartGroup(t *testing.T) {
	outputChan, outputFn := makeHandler()
	detector := NewDetector(outputFn, status.NewInfoRegistry())

	// Aggregate labels without a startGroup should not happen in practice,
	// but detector treats them as output immediately without tags
	detector.Process(newMessage("line 1"), aggregate)
	detector.Process(newMessage("line 2"), aggregate)

	msg1 := <-outputChan
	assert.Equal(t, "line 1", string(msg1.GetContent()))
	assert.Empty(t, msg1.ParsingExtra.Tags)

	msg2 := <-outputChan
	assert.Equal(t, "line 2", string(msg2.GetContent()))
	assert.Empty(t, msg2.ParsingExtra.Tags)
}

func TestDetector_ContentNotCombined(t *testing.T) {
	outputChan, outputFn := makeHandler()
	detector := NewDetector(outputFn, status.NewInfoRegistry())

	// In detection-only mode, content should NOT be combined
	detector.Process(newMessage("line 1"), startGroup)
	detector.Process(newMessage("line 2"), aggregate)
	detector.Process(newMessage("line 3"), aggregate)
	detector.Flush()

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
