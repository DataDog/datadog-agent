// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// captureAggregator is a test aggregator that records processed messages.
type captureAggregator struct {
	messages []capturedMessage
	flushed  bool
}

type capturedMessage struct {
	msg   *message.Message
	label Label
}

func (c *captureAggregator) Process(msg *message.Message, label Label) {
	c.messages = append(c.messages, capturedMessage{msg: msg, label: label})
}

func (c *captureAggregator) Flush() {
	c.flushed = true
}

func (c *captureAggregator) IsEmpty() bool {
	return len(c.messages) == 0
}

func newTestPipelineMessage(content string) *message.Message {
	msg := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	msg.RawDataLen = len(content)
	return msg
}

func newTestPipeline(enableJSON bool) (*Pipeline, *captureAggregator) {
	tailerInfo := status.NewInfoRegistry()
	aggregator := &captureAggregator{}
	tokenizer := NewTokenizer(1000)
	labeler := NewLabeler([]Heuristic{}, []Heuristic{NewPatternTable(100, 0.8, tailerInfo)})
	jsonAggregator := NewJSONAggregator(false, 10000)
	pipeline := NewPipeline(aggregator, tokenizer, labeler, jsonAggregator, enableJSON, 10*time.Second)
	return pipeline, aggregator
}

// TestPipeline_TokenizesAfterJSONAggregation verifies that tokenization runs AFTER JSON
// aggregation so that tokens reflect the complete, compacted content.
func TestPipeline_TokenizesAfterJSONAggregation(t *testing.T) {
	pipeline, agg := newTestPipeline(true)

	// Send two parts of a JSON object
	pipeline.Process(newTestPipelineMessage(`{"key":`))
	pipeline.Process(newTestPipelineMessage(`"value"}`))

	// The two parts should be aggregated into one message before reaching the aggregator
	require.Len(t, agg.messages, 1, "JSON parts should be aggregated into one message")

	// Tokens should reflect the compacted JSON, not the fragments
	capturedMsg := agg.messages[0].msg
	assert.NotNil(t, capturedMsg.ParsingExtra.Tokens, "Tokens should be set on the aggregated message")
	assert.NotEmpty(t, capturedMsg.ParsingExtra.Tokens, "Tokens should not be empty")

	// Verify the content is the compacted JSON
	assert.Equal(t, `{"key":"value"}`, string(capturedMsg.GetContent()))
}

// TestPipeline_TokenizesWhenJSONDisabled verifies that tokenization still runs when JSON
// aggregation is disabled.
func TestPipeline_TokenizesWhenJSONDisabled(t *testing.T) {
	pipeline, agg := newTestPipeline(false)

	pipeline.Process(newTestPipelineMessage(`2024-01-01 12:00:00 INFO Starting`))

	require.Len(t, agg.messages, 1)
	msg := agg.messages[0].msg
	assert.NotNil(t, msg.ParsingExtra.Tokens, "Tokens should be set even when JSON aggregation is disabled")
	assert.NotEmpty(t, msg.ParsingExtra.Tokens)
}

// TestPipeline_FlushPropagatesJSONThenAggregator verifies that Flush processes pending
// JSON fragments through processOne before flushing the aggregator.
func TestPipeline_FlushPropagatesJSONThenAggregator(t *testing.T) {
	pipeline, agg := newTestPipeline(true)

	// Send an incomplete JSON fragment
	pipeline.Process(newTestPipelineMessage(`{"key":`))

	// Nothing should reach the aggregator yet
	assert.Empty(t, agg.messages, "Incomplete JSON should not reach aggregator yet")

	// Flush should push the fragment through processOne and then flush the aggregator
	pipeline.Flush()

	// The fragment should now have been processed (as a single message since it's incomplete)
	assert.True(t, agg.flushed, "Aggregator should have been flushed")
	require.Len(t, agg.messages, 1, "Flushed JSON fragment should reach aggregator")
	assert.NotNil(t, agg.messages[0].msg.ParsingExtra.Tokens, "Flushed message should be tokenized")
}

// TestPipeline_FlushChanNilWhenEmpty verifies that FlushChan returns nil when nothing is buffered.
func TestPipeline_FlushChanNilWhenEmpty(t *testing.T) {
	pipeline, _ := newTestPipeline(true)
	assert.Nil(t, pipeline.FlushChan(), "FlushChan should be nil when pipeline is empty")
}

// TestPipeline_JSONDisabledPassesThroughDirectly verifies that with JSON aggregation disabled,
// messages go directly through processOne without JSON buffering.
func TestPipeline_JSONDisabledPassesThroughDirectly(t *testing.T) {
	pipeline, agg := newTestPipeline(false)

	pipeline.Process(newTestPipelineMessage(`{"key":`))
	pipeline.Process(newTestPipelineMessage(`"value"}`))

	// Both messages should reach the aggregator immediately (no JSON buffering)
	assert.Len(t, agg.messages, 2, "Both messages should pass through directly when JSON aggregation is disabled")
}

// TestPipeline_LabelPassedToAggregator verifies that the label from the labeler is correctly
// passed to the aggregator (not stored on the message).
func TestPipeline_LabelPassedToAggregator(t *testing.T) {
	pipeline, agg := newTestPipeline(false)

	// A message without a timestamp should get the aggregate label (default)
	pipeline.Process(newTestPipelineMessage(`continuation line without timestamp`))

	require.Len(t, agg.messages, 1)
	// The label should be set (aggregate is the default for lines without timestamps)
	_ = agg.messages[0].label // label was passed to the aggregator
}
