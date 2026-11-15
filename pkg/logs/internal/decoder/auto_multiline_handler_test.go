// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/preprocessor"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func newTestMessage(content string) *message.Message {
	msg := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	msg.RawDataLen = len(content)
	return msg
}

// newCombiningHandler creates an auto multiline handler in combining/aggregation mode.
// outputChan receives fully combined messages via a NoopSampler.
func newCombiningHandler(outputChan chan *message.Message, maxContentSize int, flushTimeout time.Duration) LineHandler {
	tailerInfo := status.NewInfoRegistry()
	tok := preprocessor.NewTokenizer(1000)
	sampler := preprocessor.NewNoopSampler()
	labeler := buildAutoMultilineLabeler(nil, nil, tailerInfo)
	aggregator := preprocessor.NewCombiningAggregator(maxContentSize, false, false, tailerInfo)
	jsonAgg := preprocessor.NewJSONAggregator(false, maxContentSize)
	return newPreprocessorHandler(aggregator, tok, labeler, sampler, outputChan, jsonAgg, preprocessor.NewNoopStackTraceAggregator(), flushTimeout, 0)
}

func TestAutoMultilineHandler_ManualFlush(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 100, 10*time.Second)

	// Add an incomplete message
	handler.process(newTestMessage(`{"key":`))

	// Manually flush
	handler.flush()

	// Check that message was flushed
	msg := <-outputChan
	assert.Equal(t, []byte(`{"key":`), msg.GetContent())
}

func TestAutoMultilineHandler_FlushWithPendingJSON(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 100, 10*time.Second)

	// Add incomplete JSON messages
	handler.process(newTestMessage(`{"key":`))
	handler.process(newTestMessage(`"value",`))

	// Flush should process all messages through jsonRecombinator
	handler.flush()

	// Verify both messages were output
	count := 0
	for i := 0; i < 2; i++ {
		<-outputChan
		count++
	}
	assert.Equal(t, 2, count)
}

func TestAutoMultilineHandler_CompleteJSONGrouping(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 1000, 10*time.Second)

	// Process complete JSON, should output immediately
	handler.process(newTestMessage(`{"key":"value"}`))

	// Verify message was processed
	msg := <-outputChan
	assert.Equal(t, []byte(`{"key":"value"}`), msg.GetContent())
}

func TestAutoMultilineHandler_MultiPartJSONGrouping(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 1000, 10*time.Second)

	// Process multi-part JSON
	handler.process(newTestMessage(`{"key":`))
	handler.process(newTestMessage(`"value"}`))

	// Should output a combined message on flush
	handler.flush()

	msg := <-outputChan
	assert.Equal(t, []byte(`{"key":"value"}`), msg.GetContent())
}

func TestAutoMultilineHandler_FlushAfterInvalidJSON(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 1000, 10*time.Second)

	// Start with valid JSON part
	handler.process(newTestMessage(`{"key":`))

	// Add invalid JSON, should cause a flush of both messages
	handler.process(newTestMessage(`invalid}`))

	// Verify both messages were output
	messages := make([]*message.Message, 0, 2)
	for i := 0; i < 2; i++ {
		messages = append(messages, <-outputChan)
	}

	assert.Equal(t, 2, len(messages))
	assert.Equal(t, []byte(`{"key":`), messages[0].GetContent())
	assert.Equal(t, []byte(`invalid}`), messages[1].GetContent())
}

func TestAutoMultilineHandler_MixedFormatLog(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 1000, 10*time.Second)

	// Process multi-part JSON
	handler.process(newTestMessage(`{"key":`))
	handler.process(newTestMessage(`"value"}`))

	// Auto flush because we found JSON
	msg := <-outputChan
	assert.Equal(t, []byte(`{"key":"value"}`), msg.GetContent())

	handler.process(newTestMessage(`10-04-2025 12:00:00 [INFO] begnning of a stack trace`))
	handler.process(newTestMessage(` at com.example.MyClass.method(MyClass.java:123)`))
	handler.process(newTestMessage(` at com.example.MyClass.method(MyClass.java:123)`))
	handler.process(newTestMessage(`10-04-2025 12:00:00 [INFO] single line log`))

	// Auto flush because we found a multiline log
	expected := []byte(`10-04-2025 12:00:00 [INFO] begnning of a stack trace\n at com.example.MyClass.method(MyClass.java:123)\n at com.example.MyClass.method(MyClass.java:123)`)
	msg = <-outputChan
	assert.Equal(t, expected, msg.GetContent())

	handler.process(newTestMessage(`10-04-2025 12:00:00 [INFO] begnning of a stack trace`))

	// Auto flush because we found a another timestamp pattern
	msg = <-outputChan
	assert.Equal(t, []byte(`10-04-2025 12:00:00 [INFO] single line log`), msg.GetContent())

	handler.process(newTestMessage(` at com.example.MyClass.method(MyClass.java:123)`))
	handler.process(newTestMessage(` at com.example.MyClass.method(MyClass.java:123)`))

	// Need to flush to clear the buffer
	select {
	case <-outputChan:
		assert.Fail(t, "Expected no more messages")
	default:
	}

	// Manual flush to clear the buffer
	handler.flush()

	msg = <-outputChan
	assert.Equal(t, expected, msg.GetContent())
}

func TestAutoMultilineHandler_JSONAggregationDisabled(t *testing.T) {
	outputChan := make(chan *message.Message, 10)

	// Create handler with JSON aggregation explicitly disabled
	tailerInfo := status.NewInfoRegistry()
	tok := preprocessor.NewTokenizer(1000)
	sampler := preprocessor.NewNoopSampler()
	labeler := buildAutoMultilineLabeler(nil, nil, tailerInfo)
	aggregator := preprocessor.NewCombiningAggregator(1000, false, false, tailerInfo)
	handler := newPreprocessorHandler(aggregator, tok, labeler, sampler, outputChan, preprocessor.NewNoopJSONAggregator(), preprocessor.NewNoopStackTraceAggregator(), 10*time.Second, 0)

	// Process multi-part JSON
	handler.process(newTestMessage(`{"key":`))
	handler.process(newTestMessage(`"value"}`))

	// Should output each message separately since JSON aggregation is disabled
	msg1 := <-outputChan
	assert.Equal(t, []byte(`{"key":`), msg1.GetContent())

	msg2 := <-outputChan
	assert.Equal(t, []byte(`"value"}`), msg2.GetContent())

	// Verify no more messages
	select {
	case <-outputChan:
		assert.Fail(t, "Expected no more messages")
	default:
	}
}

func TestAutoMultilineHandler_AggregationMode_CombinesLines(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 1000, 10*time.Second)

	// Process multiline log
	handler.process(newTestMessage(`2025-12-15 10:00:00 [ERROR] Exception occurred`))
	handler.process(newTestMessage(`  at com.example.MyClass.method1(MyClass.java:123)`))
	handler.process(newTestMessage(`  at com.example.MyClass.method2(MyClass.java:456)`))
	handler.flush()

	// In aggregation mode, lines should be combined into one message
	msg := <-outputChan
	expected := []byte(`2025-12-15 10:00:00 [ERROR] Exception occurred\n  at com.example.MyClass.method1(MyClass.java:123)\n  at com.example.MyClass.method2(MyClass.java:456)`)
	assert.Equal(t, expected, msg.GetContent())

	// Verify no more messages (combined into one)
	select {
	case <-outputChan:
		assert.Fail(t, "Expected only one combined message")
	default:
	}
}

func TestAutoMultilineHandler_CombiningMode_CombinesJSON(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	handler := newCombiningHandler(outputChan, 1000, 10*time.Second)

	// Combining mode enables JSON aggregation - split JSON should be combined
	handler.process(newTestMessage(`{"key":`))
	handler.process(newTestMessage(`"value"}`))

	// Should receive one combined message
	msg := <-outputChan
	assert.Equal(t, `{"key":"value"}`, string(msg.GetContent()))

	// Verify no more messages
	select {
	case <-outputChan:
		assert.Fail(t, "Expected only one combined message")
	default:
	}
}
