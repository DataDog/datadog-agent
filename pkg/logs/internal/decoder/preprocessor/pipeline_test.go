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

// --- test doubles ---

// captureSampler records emitted messages.
type captureSampler struct {
	emitted []*message.Message
}

func (s *captureSampler) Process(msg *message.Message) { s.emitted = append(s.emitted, msg) }
func (s *captureSampler) Flush()                       {}
func (s *captureSampler) FlushChan() <-chan time.Time  { return nil }

// captureAutoMultilineCombiner wraps autoMultilineCombiner and exposes the emitted slice.
// Used to verify that the combiner received the right messages from processOne.
type captureCombiner struct {
	received []*message.Message
}

func (c *captureCombiner) Process(msg *message.Message, _ Label) []*message.Message {
	c.received = append(c.received, msg)
	return []*message.Message{msg}
}
func (c *captureCombiner) Flush() []*message.Message { return nil }
func (c *captureCombiner) IsEmpty() bool             { return true }

// flushCaptureCombiner tracks flush calls.
type flushCaptureCombiner struct {
	flushed bool
	pending *message.Message
}

func (c *flushCaptureCombiner) Process(msg *message.Message, _ Label) []*message.Message {
	c.pending = msg
	return nil // buffer the message
}

func (c *flushCaptureCombiner) Flush() []*message.Message {
	c.flushed = true
	if c.pending != nil {
		msg := c.pending
		c.pending = nil
		return []*message.Message{msg}
	}
	return nil
}

func (c *flushCaptureCombiner) IsEmpty() bool {
	return c.pending == nil
}

// --- helpers ---

func newTestPipelineMessage(content string) *message.Message {
	msg := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	msg.RawDataLen = len(content)
	return msg
}

func newTestPipeline(enableJSON bool) (*Pipeline, *captureCombiner, *captureSampler) {
	tailerInfo := status.NewInfoRegistry()
	combiner := &captureCombiner{}
	sampler := &captureSampler{}
	tokenizer := NewTokenizer(1000)
	jsonAggregator := NewJSONAggregator(false, 10000)
	// PatternTable is needed so the pipeline is representative, but combiner is captured above
	_ = tailerInfo
	pipeline := NewPipeline(combiner, tokenizer, nil, sampler, jsonAggregator, enableJSON, 10*time.Second)
	return pipeline, combiner, sampler
}

// --- tests ---

// TestPipeline_CombinerReceivesMessage verifies that the combiner receives the message
// after tokenization and labeling have run (tokens are local to the pipeline, not on the message).
func TestPipeline_CombinerReceivesMessage(t *testing.T) {
	pipeline, combiner, _ := newTestPipeline(false)

	pipeline.Process(newTestPipelineMessage(`2024-01-01 12:00:00 INFO Starting`))

	require.Len(t, combiner.received, 1)
	assert.Equal(t, `2024-01-01 12:00:00 INFO Starting`, string(combiner.received[0].GetContent()))
}

// TestPipeline_TokenizesAfterJSONAggregation verifies that the combiner sees the combined
// JSON content (not individual fragments), confirming JSON aggregation runs before combining.
func TestPipeline_TokenizesAfterJSONAggregation(t *testing.T) {
	pipeline, combiner, _ := newTestPipeline(true)

	// Two fragments that together form a complete JSON object
	pipeline.Process(newTestPipelineMessage(`{"key":`))
	pipeline.Process(newTestPipelineMessage(`"value"}`))

	// JSON aggregation should combine the two fragments into one before the combiner sees it
	require.Len(t, combiner.received, 1, "JSON parts should be aggregated into one message")
	assert.Equal(t, `{"key":"value"}`, string(combiner.received[0].GetContent()))
}

// TestPipeline_JSONDisabledPassesThroughDirectly verifies that with JSON aggregation disabled,
// messages go directly to processOne without JSON buffering.
func TestPipeline_JSONDisabledPassesThroughDirectly(t *testing.T) {
	pipeline, combiner, _ := newTestPipeline(false)

	pipeline.Process(newTestPipelineMessage(`{"key":`))
	pipeline.Process(newTestPipelineMessage(`"value"}`))

	assert.Len(t, combiner.received, 2, "Both messages should pass directly when JSON aggregation is disabled")
}

// TestPipeline_FlushCascadesInOrder verifies that Flush processes pending JSON fragments
// through processOne, then flushes the combiner, then calls sampler.Flush.
func TestPipeline_FlushCascadesInOrder(t *testing.T) {
	tailerInfo := status.NewInfoRegistry()
	_ = tailerInfo
	combiner := &flushCaptureCombiner{}
	sampler := &captureSampler{}
	tokenizer := NewTokenizer(1000)
	jsonAggregator := NewJSONAggregator(false, 10000)
	pipeline := NewPipeline(combiner, tokenizer, nil, sampler, jsonAggregator, true, 10*time.Second)

	// Send an incomplete JSON fragment — it stays in jsonAggregator
	pipeline.Process(newTestPipelineMessage(`{"key":`))
	assert.Nil(t, combiner.pending, "Combiner should not see incomplete JSON yet")

	// Flush must: (1) push JSON fragment to processOne → combiner.Process, (2) combiner.Flush → sampler
	pipeline.Flush()

	assert.True(t, combiner.flushed, "Combiner should have been flushed")
	// The JSON fragment went through processOne (combiner.Process returned nil),
	// then combiner.Flush returned it, so sampler should have received it
	require.Len(t, sampler.emitted, 1, "Sampler should have received the flushed message")
	assert.Equal(t, `{"key":`, string(sampler.emitted[0].GetContent()))
}

// TestPipeline_FlushChanNilWhenEmpty verifies that FlushChan returns nil when nothing is buffered.
func TestPipeline_FlushChanNilWhenEmpty(t *testing.T) {
	pipeline, _, _ := newTestPipeline(true)
	assert.Nil(t, pipeline.FlushChan(), "FlushChan should be nil when pipeline is empty")
}

// TestPipeline_SamplerReceivesCompletedMessages verifies that messages returned by the
// combiner flow to the sampler.
func TestPipeline_SamplerReceivesCompletedMessages(t *testing.T) {
	pipeline, _, sampler := newTestPipeline(false)

	pipeline.Process(newTestPipelineMessage(`hello world`))

	require.Len(t, sampler.emitted, 1, "Sampler should receive the message that combiner returned")
	assert.Equal(t, `hello world`, string(sampler.emitted[0].GetContent()))
}
