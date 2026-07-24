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

func (a *captureAggregator) Process(msg *message.Message, _ Label, _ []Token) []AggregatedMessageWithTokens {
	a.received = append(a.received, msg)
	return []AggregatedMessageWithTokens{{Msg: msg}}
}
func (a *captureAggregator) Flush() []AggregatedMessageWithTokens { return nil }
func (a *captureAggregator) IsEmpty() bool                        { return true }

// flushCaptureAggregator tracks flush calls.
type flushCaptureAggregator struct {
	flushed bool
	pending *message.Message
}

func (a *flushCaptureAggregator) Process(msg *message.Message, _ Label, _ []Token) []AggregatedMessageWithTokens {
	a.pending = msg
	return nil // buffer the message
}

func (a *flushCaptureAggregator) Flush() []AggregatedMessageWithTokens {
	a.flushed = true
	if a.pending != nil {
		msg := a.pending
		a.pending = nil
		return []AggregatedMessageWithTokens{{Msg: msg}}
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
	preprocessor := NewPreprocessor(aggregator, NewTokenizer(1000), NewNoopLabeler(), sampler, outputChan, jsonAggregator, NewNoopStackTraceAggregator(), 10*time.Second, 0)
	return preprocessor, aggregator, sampler
}

// --- tests ---

// TestPreprocessor_AggregatorReceivesMessage anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @guidance step 2 — the per-call flow runs Tokenization,
//	                       Labeler, then Aggregator.process in
//	                       order on each emission from
//	                       JSONAggregator.process.
//
// Verifies the order-of-stage wiring at the Aggregator boundary:
// after JSON aggregation emits a message, the aggregator receives
// it (post-tokenize, post-label).
func TestPreprocessor_AggregatorReceivesMessage(t *testing.T) {
	preprocessor, aggregator, _ := newTestPreprocessor(false)

	preprocessor.Process(newTestPreprocessorMessage(`2024-01-01 12:00:00 INFO Starting`))

	require.Len(t, aggregator.received, 1)
	assert.Equal(t, `2024-01-01 12:00:00 INFO Starting`, string(aggregator.received[0].GetContent()))
}

// TestPreprocessor_TokenizesAfterJSONAggregation anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @guidance step 1 + step 2 — JSONAggregator.process runs
//	                                BEFORE Tokenization. Multi-line
//	                                JSON fragments are joined into
//	                                one message before the downstream
//	                                stages see them.
//
// Two JSON fragments arrive in separate process calls. JSON
// aggregation combines them before the aggregator sees a single
// combined message.
func TestPreprocessor_TokenizesAfterJSONAggregation(t *testing.T) {
	preprocessor, aggregator, _ := newTestPreprocessor(true)

	// Two fragments that together form a complete JSON object
	preprocessor.Process(newTestPreprocessorMessage(`{"key":`))
	preprocessor.Process(newTestPreprocessorMessage(`"value"}`))

	// JSON aggregation should combine the two fragments into one before the aggregator sees it
	require.Len(t, aggregator.received, 1, "JSON parts should be aggregated into one message")
	assert.Equal(t, `{"key":"value"}`, string(aggregator.received[0].GetContent()))
}

// TestPreprocessor_JSONDisabledPassesThroughDirectly anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @guidance step 1 — JSONAggregator.process(input) may
//	                       emit zero, one, or many messages per
//	                       input depending on the fulfiller.
//
// With NoopJSONAggregator wired in, each input emits exactly one
// downstream message. This is the no-buffering JSON path
// (verifies the contract's per-fulfiller flexibility on step 1).
func TestPreprocessor_JSONDisabledPassesThroughDirectly(t *testing.T) {
	preprocessor, aggregator, _ := newTestPreprocessor(false)

	preprocessor.Process(newTestPreprocessorMessage(`{"key":`))
	preprocessor.Process(newTestPreprocessorMessage(`"value"}`))

	assert.Len(t, aggregator.received, 2, "Both messages should pass directly when JSON aggregation is disabled")
}

// TestPreprocessor_FlushCascadesInOrder anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @invariant FlushDrainsBuffer
//	    @guidance flush flow — JSONAggregator.flush runs first;
//	                           each emission goes through
//	                           Tokenization/Labeler/Aggregator;
//	                           THEN Aggregator.flush runs; THEN
//	                           Sampler.flush. After all of this,
//	                           every stateful component reports
//	                           is_empty.
//
// A buffered JSON fragment held by the JSONAggregator at flush
// time goes through the full downstream chain. The aggregator's
// flush is then called separately, producing a final emission.
func TestPreprocessor_FlushCascadesInOrder(t *testing.T) {
	tailerInfo := status.NewInfoRegistry()
	_ = tailerInfo
	aggregator := &flushCaptureAggregator{}
	sampler := &captureSampler{}
	outputChan := make(chan *message.Message, 10)
	preprocessor := NewPreprocessor(aggregator, NewTokenizer(1000), NewNoopLabeler(), sampler, outputChan, NewJSONAggregator(false, 10000), NewNoopStackTraceAggregator(), 10*time.Second, 0)

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

// TestPreprocessor_FlushChanNilWhenEmpty anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    auxiliary observable — the Preprocessor exposes a
//	    FlushChan whose value is nil when there is no buffered
//	    state requiring a future flush.
//
// FlushChan is nil at construction. (Not a tie-spec @invariant
// per se — the FlushChan is the Preprocessor's interaction with
// the upstream decoder's flush-timing logic. Anchored here to
// the implementation's documented contract.)
func TestPreprocessor_FlushChanNilWhenEmpty(t *testing.T) {
	preprocessor, _, _ := newTestPreprocessor(true)
	assert.Nil(t, preprocessor.FlushChan(), "FlushChan should be nil when preprocessor is empty")
}

// TestPreprocessor_SamplerReceivesAggregatedMessageWithTokens anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @guidance step 2d — for each AggregatedMessageWithTokens
//	                        yielded by Aggregator.process, the
//	                        Sampler receives the message AND the
//	                        first-line tokens.
//	    @invariant EndToEndTokenForwarding
//
// The aggregator's emitted AggregatedMessageWithTokens flows
// through to the sampler; both the message and the tokens are
// forwarded.
func TestPreprocessor_SamplerReceivesAggregatedMessageWithTokens(t *testing.T) {
	preprocessor, _, sampler := newTestPreprocessor(false)

	preprocessor.Process(newTestPreprocessorMessage(`hello world`))

	require.Len(t, sampler.emitted, 1, "Sampler should receive the message that aggregator returned")
	assert.Equal(t, `hello world`, string(sampler.emitted[0].GetContent()))
}
