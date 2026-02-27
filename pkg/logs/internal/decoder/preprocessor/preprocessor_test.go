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

func (s *captureSampler) Process(msg *message.Message, _ []Token) *message.Message {
	s.emitted = append(s.emitted, msg)
	return msg
}
func (s *captureSampler) Flush() *message.Message { return nil }

// captureAggregator wraps an aggregator and exposes the received slice.
// Used to verify that the aggregator received the right messages from processOne.
type captureAggregator struct {
	received []*message.Message
}

func (a *captureAggregator) Process(msg *message.Message, _ Label, _ []Token) []CompletedMessage {
	a.received = append(a.received, msg)
	return []CompletedMessage{{Msg: msg}}
}
func (a *captureAggregator) Flush() []CompletedMessage { return nil }
func (a *captureAggregator) IsEmpty() bool             { return true }

// flushCaptureAggregator tracks flush calls.
type flushCaptureAggregator struct {
	flushed bool
	pending *message.Message
}

func (a *flushCaptureAggregator) Process(msg *message.Message, _ Label, _ []Token) []CompletedMessage {
	a.pending = msg
	return nil // buffer the message
}

func (a *flushCaptureAggregator) Flush() []CompletedMessage {
	a.flushed = true
	if a.pending != nil {
		msg := a.pending
		a.pending = nil
		return []CompletedMessage{{Msg: msg}}
	}
	return nil
}

func (a *flushCaptureAggregator) IsEmpty() bool {
	return a.pending == nil
}

// --- helpers ---

func newTestPreprocessorMessage(content string) *message.Message {
	msg := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	msg.RawDataLen = len(content)
	return msg
}

func newTestPreprocessor(enableJSON bool) (*Preprocessor, *captureAggregator, *captureSampler) {
	tailerInfo := status.NewInfoRegistry()
	aggregator := &captureAggregator{}
	sampler := &captureSampler{}
	_ = tailerInfo
	var jsonAggregator JSONAggregator = NewNoopJSONAggregator()
	if enableJSON {
		jsonAggregator = NewJSONAggregator(false, 10000)
	}
	outputChan := make(chan *message.Message, 10)
	preprocessor := NewPreprocessor(aggregator, NewTokenizer(1000), NewNoopLabeler(), sampler, outputChan, jsonAggregator, 10*time.Second)
	return preprocessor, aggregator, sampler
}

// --- tests ---

// TestPreprocessor_AggregatorReceivesMessage verifies that the aggregator receives the message
// after tokenization and labeling have run (tokens are local to the preprocessor, not on the message).
func TestPreprocessor_AggregatorReceivesMessage(t *testing.T) {
	preprocessor, aggregator, _ := newTestPreprocessor(false)

	preprocessor.Process(newTestPreprocessorMessage(`2024-01-01 12:00:00 INFO Starting`))

	require.Len(t, aggregator.received, 1)
	assert.Equal(t, `2024-01-01 12:00:00 INFO Starting`, string(aggregator.received[0].GetContent()))
}

// TestPreprocessor_TokenizesAfterJSONAggregation verifies that the aggregator sees the combined
// JSON content (not individual fragments), confirming JSON aggregation runs before aggregation.
func TestPreprocessor_TokenizesAfterJSONAggregation(t *testing.T) {
	preprocessor, aggregator, _ := newTestPreprocessor(true)

	// Two fragments that together form a complete JSON object
	preprocessor.Process(newTestPreprocessorMessage(`{"key":`))
	preprocessor.Process(newTestPreprocessorMessage(`"value"}`))

	// JSON aggregation should combine the two fragments into one before the aggregator sees it
	require.Len(t, aggregator.received, 1, "JSON parts should be aggregated into one message")
	assert.Equal(t, `{"key":"value"}`, string(aggregator.received[0].GetContent()))
}

// TestPreprocessor_JSONDisabledPassesThroughDirectly verifies that with JSON aggregation disabled,
// messages go directly to processOne without JSON buffering.
func TestPreprocessor_JSONDisabledPassesThroughDirectly(t *testing.T) {
	preprocessor, aggregator, _ := newTestPreprocessor(false)

	preprocessor.Process(newTestPreprocessorMessage(`{"key":`))
	preprocessor.Process(newTestPreprocessorMessage(`"value"}`))

	assert.Len(t, aggregator.received, 2, "Both messages should pass directly when JSON aggregation is disabled")
}

// TestPreprocessor_FlushCascadesInOrder verifies that Flush processes pending JSON fragments
// through processOne, then flushes the aggregator, then calls sampler.Flush.
func TestPreprocessor_FlushCascadesInOrder(t *testing.T) {
	tailerInfo := status.NewInfoRegistry()
	_ = tailerInfo
	aggregator := &flushCaptureAggregator{}
	sampler := &captureSampler{}
	outputChan := make(chan *message.Message, 10)
	preprocessor := NewPreprocessor(aggregator, NewTokenizer(1000), NewNoopLabeler(), sampler, outputChan, NewJSONAggregator(false, 10000), 10*time.Second)

	// Send an incomplete JSON fragment — it stays in jsonAggregator
	preprocessor.Process(newTestPreprocessorMessage(`{"key":`))
	assert.Nil(t, aggregator.pending, "Aggregator should not see incomplete JSON yet")

	// Flush must: (1) push JSON fragment to processOne → aggregator.Process, (2) aggregator.Flush → sampler
	preprocessor.Flush()

	assert.True(t, aggregator.flushed, "Aggregator should have been flushed")
	// The JSON fragment went through processOne (aggregator.Process returned nil),
	// then aggregator.Flush returned it, so sampler should have received it
	require.Len(t, sampler.emitted, 1, "Sampler should have received the flushed message")
	assert.Equal(t, `{"key":`, string(sampler.emitted[0].GetContent()))
}

// TestPreprocessor_FlushChanNilWhenEmpty verifies that FlushChan returns nil when nothing is buffered.
func TestPreprocessor_FlushChanNilWhenEmpty(t *testing.T) {
	preprocessor, _, _ := newTestPreprocessor(true)
	assert.Nil(t, preprocessor.FlushChan(), "FlushChan should be nil when preprocessor is empty")
}

// TestPreprocessor_SamplerReceivesCompletedMessages verifies that messages returned by the
// aggregator flow to the sampler.
func TestPreprocessor_SamplerReceivesCompletedMessages(t *testing.T) {
	preprocessor, _, sampler := newTestPreprocessor(false)

	preprocessor.Process(newTestPreprocessorMessage(`hello world`))

	require.Len(t, sampler.emitted, 1, "Sampler should receive the message that aggregator returned")
	assert.Equal(t, `hello world`, string(sampler.emitted[0].GetContent()))
}
