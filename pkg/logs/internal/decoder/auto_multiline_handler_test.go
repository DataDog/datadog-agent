// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func newTestMessage(content string) *message.Message {
	msg := message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
	msg.RawDataLen = len(content)
	return msg
}

func TestAutoMultilineHandler_ManualFlush(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with long flush timeout to avoid auto-flush during test
	handler := NewAutoMultilineHandler(outputFn, 100, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)

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
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with long flush timeout
	handler := NewAutoMultilineHandler(outputFn, 100, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)

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
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)

	// Process complete JSON, should output immediately
	handler.process(newTestMessage(`{"key":"value"}`))

	// Verify message was processed
	msg := <-outputChan
	assert.Equal(t, []byte(`{"key":"value"}`), msg.GetContent())
}

func TestAutoMultilineHandler_MultiPartJSONGrouping(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)

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
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)

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
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)

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
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with JSON aggregation disabled
	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)
	handler.enableJSONAggregation = false

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

func TestAutoMultilineHandler_DetectionOnlyMode_SingleLineNotTagged(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with detection-only mode enabled
	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, true)

	// Process single-line logs
	handler.process(newTestMessage(`2025-12-15 10:00:00 [INFO] Single line log`))
	handler.process(newTestMessage(`{"key":"value"}`))
	handler.flush()

	// Verify single-line logs are NOT tagged
	msg1 := <-outputChan
	assert.Equal(t, []byte(`2025-12-15 10:00:00 [INFO] Single line log`), msg1.GetContent())
	assert.Empty(t, msg1.ProcessingTags, "Single-line log should not have processing tags")

	msg2 := <-outputChan
	assert.Equal(t, []byte(`{"key":"value"}`), msg2.GetContent())
	assert.Empty(t, msg2.ProcessingTags, "Single-line log should not have processing tags")

	// Verify no more messages
	select {
	case <-outputChan:
		assert.Fail(t, "Expected no more messages")
	default:
	}
}

func TestAutoMultilineHandler_DetectionOnlyMode_MultilineTagged(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with detection-only mode enabled
	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, true)

	// Process multiline log (stack trace)
	handler.process(newTestMessage(`2025-12-15 10:00:00 [ERROR] Exception occurred`))
	handler.process(newTestMessage(`  at com.example.MyClass.method1(MyClass.java:123)`))
	handler.process(newTestMessage(`  at com.example.MyClass.method2(MyClass.java:456)`))
	handler.process(newTestMessage(`2025-12-15 10:00:01 [INFO] Next log`)) // Triggers flush of multiline group
	handler.flush()                                                        // Flush the last single-line message

	// Verify all 3 lines of the multiline group are output separately with tags
	msg1 := <-outputChan
	assert.Equal(t, []byte(`2025-12-15 10:00:00 [ERROR] Exception occurred`), msg1.GetContent())
	assert.Contains(t, msg1.ProcessingTags, "auto_multiline_group_size:3", "First line should be tagged with group size")

	msg2 := <-outputChan
	assert.Equal(t, []byte(`  at com.example.MyClass.method1(MyClass.java:123)`), msg2.GetContent())
	assert.Contains(t, msg2.ProcessingTags, "auto_multiline_group_size:3", "Second line should be tagged with group size")

	msg3 := <-outputChan
	assert.Equal(t, []byte(`  at com.example.MyClass.method2(MyClass.java:456)`), msg3.GetContent())
	assert.Contains(t, msg3.ProcessingTags, "auto_multiline_group_size:3", "Third line should be tagged with group size")

	// Next single-line log should not be tagged
	msg4 := <-outputChan
	assert.Equal(t, []byte(`2025-12-15 10:00:01 [INFO] Next log`), msg4.GetContent())
	assert.Empty(t, msg4.ProcessingTags, "Single-line log should not have processing tags")
}

func TestAutoMultilineHandler_DetectionOnlyMode_TwoLineGroup(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with detection-only mode enabled
	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, true)

	// Process 2-line multiline log
	handler.process(newTestMessage(`2025-12-15 10:00:00 [ERROR] Error message`))
	handler.process(newTestMessage(`  continuation line`))
	handler.flush()

	// Verify both lines are tagged with group size 2
	msg1 := <-outputChan
	assert.Equal(t, []byte(`2025-12-15 10:00:00 [ERROR] Error message`), msg1.GetContent())
	assert.Contains(t, msg1.ProcessingTags, "auto_multiline_group_size:2")

	msg2 := <-outputChan
	assert.Equal(t, []byte(`  continuation line`), msg2.GetContent())
	assert.Contains(t, msg2.ProcessingTags, "auto_multiline_group_size:2")

	// Verify no more messages
	select {
	case <-outputChan:
		assert.Fail(t, "Expected no more messages")
	default:
	}
}

func TestAutoMultilineHandler_DetectionOnlyMode_MixedLogs(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with detection-only mode enabled
	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, true)

	// Mix of single-line and multiline logs
	handler.process(newTestMessage(`2025-12-15 10:00:00 [INFO] Single line`))
	handler.process(newTestMessage(`2025-12-15 10:00:01 [ERROR] Start of multiline`))
	handler.process(newTestMessage(`  continuation`))
	handler.process(newTestMessage(`2025-12-15 10:00:02 [INFO] Another single line`))
	handler.flush()

	// First single line - not tagged
	msg1 := <-outputChan
	assert.Equal(t, []byte(`2025-12-15 10:00:00 [INFO] Single line`), msg1.GetContent())
	assert.Empty(t, msg1.ProcessingTags)

	// Multiline group - both lines tagged
	msg2 := <-outputChan
	assert.Equal(t, []byte(`2025-12-15 10:00:01 [ERROR] Start of multiline`), msg2.GetContent())
	assert.Contains(t, msg2.ProcessingTags, "auto_multiline_group_size:2")

	msg3 := <-outputChan
	assert.Equal(t, []byte(`  continuation`), msg3.GetContent())
	assert.Contains(t, msg3.ProcessingTags, "auto_multiline_group_size:2")

	// Second single line - not tagged
	msg4 := <-outputChan
	assert.Equal(t, []byte(`2025-12-15 10:00:02 [INFO] Another single line`), msg4.GetContent())
	assert.Empty(t, msg4.ProcessingTags)
}

func TestAutoMultilineHandler_AggregationMode_CombinesLines(t *testing.T) {
	outputChan := make(chan *message.Message, 10)
	outputFn := func(m *message.Message) {
		outputChan <- m
	}

	// Create handler with aggregation mode (isDetectionOnly=false)
	handler := NewAutoMultilineHandler(outputFn, 1000, 10*time.Second, status.NewInfoRegistry(), nil, nil, false)

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
