// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

func newTestMessage(content string) *message.Message {
	return message.NewMessage([]byte(content), nil, message.StatusInfo, 0)
}

func TestJSONAggregatorProcess_Complete(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Single complete JSON message
	msg := newTestMessage(`{"key":"value"}`)
	result := aggregator.Process(msg)

	assert.Equal(t, 1, len(result), "Expected one message to be returned")
	assert.Equal(t, []byte(`{"key":"value"}`), result[0].GetContent(), "Content should be unchanged for complete JSON")
}

func TestJSONAggregatorProcess_InvalidSingleLineJSON(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Invalid JSON with balanced braces (missing quotes around keys)
	// This tests the case where isSingleLineJSON() returns true but json.Valid() returns false
	msg := newTestMessage(`{timestamp:"2024-01-01",level:info,message:"test"}`)
	result := aggregator.Process(msg)

	// Should return 1 message (flushed as invalid)
	assert.Equal(t, 1, len(result), "Expected one message to be returned for invalid JSON")
	// Content should be unchanged since it's invalid and gets flushed
	assert.Equal(t, []byte(`{timestamp:"2024-01-01",level:info,message:"test"}`), result[0].GetContent(), "Invalid JSON should be returned unchanged")
}

func TestJSONAggregatorProcess_Incomplete(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Incomplete JSON message
	msg := newTestMessage(`{"key":`)
	result := aggregator.Process(msg)

	assert.Equal(t, 0, len(result), "Expected no messages to be returned for incomplete JSON")
}

func TestJSONAggregatorProcess_MultiPart(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// First part of a JSON message
	msg1 := newTestMessage(`{"key":`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for first incomplete part")

	// Second part completes the JSON
	msg2 := newTestMessage(`"value"}`)
	result = aggregator.Process(msg2)

	assert.Equal(t, 1, len(result), "Expected one message after completion")
	assert.Equal(t, []byte(`{"key":"value"}`), result[0].GetContent(), "Content should be compact JSON")
}

func TestJSONAggregatorProcess_MultiPart_RawDataLen(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)
	part1 := `{"key":        `
	part2 := `      "value"}       `
	expectedRawDataLen := len([]byte(part1)) + len([]byte(part2))

	// First part of a JSON message
	msg1 := newTestMessage(part1)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for first incomplete part")

	// Second part completes the JSON
	msg2 := newTestMessage(part2)
	result = aggregator.Process(msg2)

	assert.Equal(t, 1, len(result), "Expected one message after completion")
	assert.Equal(t, []byte(`{"key":"value"}`), result[0].GetContent(), "Content should be compact JSON")
	assert.Equal(t, expectedRawDataLen, result[0].RawDataLen, "Expected raw data length to be the sum of the two parts")
}

func TestJSONAggregatorProcess_MultiPart_RawDataLen_original_size_differs(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)
	part1 := "{\"key\":        \n"
	part2 := "      \"value\"}       \n"
	expectedRawDataLen := len([]byte(part1)) + len([]byte(part2))

	// First part of a JSON message with the newline stripped
	msg1 := newTestMessage(strings.ReplaceAll(part1, "\n", ""))
	// The original size retains the byte offset of the newline
	msg1.RawDataLen = len([]byte(part1))
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for first incomplete part")

	// Second part completes the JSON with the newline stripped
	msg2 := newTestMessage(strings.ReplaceAll(part2, "\n", ""))
	// The original size retains the byte offset of the newline
	msg2.RawDataLen = len([]byte(part2))
	result = aggregator.Process(msg2)

	assert.Equal(t, 1, len(result), "Expected one message after completion")
	assert.Equal(t, []byte(`{"key":"value"}`), result[0].GetContent(), "Content should be compact JSON")
	assert.Equal(t, expectedRawDataLen, result[0].RawDataLen, "Expected raw data length to be the sum of the two parts")
}

func TestJSONAggregatorProcess_Invalid(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// First part valid
	msg1 := newTestMessage(`{"key":`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for incomplete part")

	// Second part invalid
	msg2 := newTestMessage(`invalid}`)
	result = aggregator.Process(msg2)

	assert.Equal(t, 2, len(result), "Expected original messages to be returned for invalid JSON")
	assert.Equal(t, []byte(`{"key":`), result[0].GetContent(), "First original message should be returned")
	assert.Equal(t, []byte(`invalid}`), result[1].GetContent(), "Second original message should be returned")
}

func TestJSONAggregatorFlush(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Buffer some incomplete JSON
	msg1 := newTestMessage(`{"key":`)
	msg2 := newTestMessage(`"value",`)

	aggregator.Process(msg1)
	aggregator.Process(msg2)

	// Flush and verify all messages are returned
	result := aggregator.Flush()

	assert.Equal(t, 2, len(result), "Expected all buffered messages to be returned")
	assert.Equal(t, []byte(`{"key":`), result[0].GetContent(), "First message content should match")
	assert.Equal(t, []byte(`"value",`), result[1].GetContent(), "Second message content should match")

	// Verify buffer is cleared after flush
	emptyResult := aggregator.Flush()
	assert.Equal(t, 0, len(emptyResult), "Expected empty result after flushing")
}

func TestJSONAggregatorMaxSize(t *testing.T) {
	// Set a small max size to test size limits
	aggregator := NewJSONAggregator(true, 10)

	// First part within size limit
	msg1 := newTestMessage(`{"key":`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for first incomplete part")

	// Second part exceeds size limit
	msg2 := newTestMessage(`"very long value that exceeds the size limit"}`)
	result = aggregator.Process(msg2)

	// Should flush both messages since size limit was exceeded
	assert.Equal(t, 2, len(result), "Expected both messages to be returned when size limit exceeded")
	assert.Equal(t, []byte(`{"key":`), result[0].GetContent(), "First message content should match")
	assert.Equal(t, []byte(`"very long value that exceeds the size limit"}`), result[1].GetContent(), "Second message content should match")

	// Verify buffer is cleared after size limit flush
	emptyResult := aggregator.Flush()
	assert.Equal(t, 0, len(emptyResult), "Expected empty result after size limit flush")
}

func TestJSONAggregatorTelemetry(t *testing.T) {
	aggregator := NewJSONAggregator(true, 100)
	initialTrue := metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("true").Get()
	initialFalse := metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("false").Get()

	// A full single line JSON message should not have the aggregated JSON tag
	msg1 := newTestMessage(`{"key":"value"}`)
	result := aggregator.Process(msg1)
	assert.NotContains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag)
	assert.Equal(t, initialTrue, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("true").Get())
	assert.Equal(t, initialFalse, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("false").Get())

	// an aggregated multiline JSON message should have the aggregated JSON tag
	msg2 := newTestMessage(`{"key":`)
	msg3 := newTestMessage(`"value"}`)
	_ = aggregator.Process(msg2)
	result = aggregator.Process(msg3)

	assert.Contains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag)
	assert.Equal(t, initialTrue+1, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("true").Get())
	assert.Equal(t, initialFalse, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("false").Get())

	// Partially valid JSON
	msg4 := newTestMessage(`{"key":`)
	_ = aggregator.Process(msg4)
	msg5 := newTestMessage(`Not a JSON object`)
	result = aggregator.Process(msg5)

	assert.NotContains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag)
	assert.Equal(t, initialTrue+1, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("true").Get())
	// increment because we had a partially valid JSON object that was later invalidated
	assert.Equal(t, initialFalse+1, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("false").Get())

	// Totally invalid JSON
	msg6 := newTestMessage(`Not a JSON object`)
	result = aggregator.Process(msg6)

	assert.NotContains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag)
	assert.Equal(t, initialTrue+1, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("true").Get())
	// Should not increment because we had a totally invalid JSON object
	assert.Equal(t, initialFalse+1, metrics.TlmAutoMultilineJSONAggregatorFlush.WithValues("false").Get())

}

func TestHasBalancedBraces(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "simple single-line JSON",
			input:    `{"key":"value"}`,
			expected: true,
		},
		{
			name:     "single-line JSON with trailing whitespace",
			input:    `{"key":"value"}  `,
			expected: true,
		},
		{
			name:     "single-line JSON with trailing newline",
			input:    `{"key":"value"}` + "\n",
			expected: true,
		},
		{
			name:     "incomplete JSON - unbalanced braces",
			input:    `{"key":"value"`,
			expected: false,
		},
		{
			name:     "JSON with nested objects",
			input:    `{"outer":{"inner":"value"}}`,
			expected: true,
		},
		{
			name:     "JSON with escaped quotes",
			input:    `{"key":"value with \"quotes\""}`,
			expected: true,
		},
		{
			name:     "JSON with brace in string",
			input:    `{"key":"value with } brace"}`,
			expected: true,
		},
		{
			name:     "JSON with trailing garbage",
			input:    `{"key":"value"} garbage`,
			expected: false,
		},
		{
			name:     "empty string",
			input:    ``,
			expected: false,
		},
		{
			name:     "not starting with brace",
			input:    `not json`,
			expected: false,
		},
		{
			name:     "valid JSON with escaped JSON in value",
			input:    `{"data":"{\"inner\":\"value\"}"}`,
			expected: true,
		},
		{
			name:     "valid JSON with escaped JSON containing braces in value",
			input:    `{"log":"{\"message\":\"error: { not a problem }\",\"level\":\"info\"}"}`,
			expected: true,
		},
		{
			name:     "valid JSON with escaped JSON and real nested object",
			input:    `{"outer":{"stringified":"{\"inner\":\"value\"}"},"other":"data"}`,
			expected: true,
		},
		{
			name:     "invalid - incomplete outer JSON with escaped JSON inside",
			input:    `{"data":"{\"inner\":\"value\"}"`,
			expected: false,
		},
		{
			name:     "invalid - complete outer JSON but malformed escaped JSON string",
			input:    `{"data":"{\"inner\":\"value}"}`,
			expected: true, // The outer JSON is balanced, inner escaped JSON validity doesn't matter for brace counting
		},
		{
			name:     "valid JSON with deeply escaped JSON",
			input:    `{"data":"{\\\"nested\\\":\\\"{\\\\\\\"deep\\\\\\\":\\\\\\\"value\\\\\\\"}\\\"}"}`,
			expected: true,
		},
		{
			name:     "valid JSON with escaped backslash before quote",
			input:    `{"path":"C:\\\"Program Files\\\"\\test.json"}`,
			expected: true,
		},
		{
			name:     "valid JSON with escaped JSON containing escaped quotes",
			input:    `{"log":"{\"msg\":\"He said \\\"hello\\\"\"}"}`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// assess that json.Valid() returns the expected result to detect single line JSON
			result := json.Valid([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONAggregatorFastPath(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Fast path should handle single-line JSON without full parsing
	msg := newTestMessage(`{"key":"value","number":42,"nested":{"inner":"data"}}`)
	result := aggregator.Process(msg)

	assert.Equal(t, 1, len(result), "Expected one message to be returned")
	assert.Equal(t, []byte(`{"key":"value","number":42,"nested":{"inner":"data"}}`), result[0].GetContent(), "Content should be unchanged")
	assert.NotContains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag, "Should not be tagged as aggregated")

	// Verify the decoder buffer is still empty (fast path bypassed it)
	assert.True(t, aggregator.IsEmpty(), "Aggregator buffer should be empty after fast path")
}

func TestJSONAggregatorFastPathWithTrailingWhitespace(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Single-line JSON with trailing whitespace should use fast path
	msg := newTestMessage(`{"key":"value"}   ` + "\n\t")
	result := aggregator.Process(msg)

	assert.Equal(t, 1, len(result), "Expected one message to be returned")
	assert.True(t, aggregator.IsEmpty(), "Aggregator buffer should be empty after fast path")
}

func TestJSONAggregatorMultilineStillWorks(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Multiline JSON should still aggregate properly
	msg1 := newTestMessage(`{"key":`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for incomplete JSON")

	msg2 := newTestMessage(`"value"}`)
	result = aggregator.Process(msg2)
	assert.Equal(t, 1, len(result), "Expected one message after completion")
	assert.Equal(t, []byte(`{"key":"value"}`), result[0].GetContent(), "Content should be compacted")
	assert.Contains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag, "Should be tagged as aggregated")
}

func TestJSONAggregatorRootLevelArray(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	msg1 := newTestMessage(`[`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for incomplete array")

	msg2 := newTestMessage(`{"key": "val"},`)
	result = aggregator.Process(msg2)
	assert.Equal(t, 0, len(result), "Expected no messages for incomplete array")

	msg3 := newTestMessage(`{"key": "val2"}`)
	result = aggregator.Process(msg3)
	assert.Equal(t, 0, len(result), "Expected no messages for incomplete array")

	msg4 := newTestMessage(`]`)
	result = aggregator.Process(msg4)
	assert.Equal(t, 1, len(result), "Expected one aggregated message")
	assert.Equal(t, []byte(`[{"key":"val"},{"key":"val2"}]`), result[0].GetContent())
	assert.Contains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag)
}

func TestJSONAggregatorRootLevelArraySingleLine(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	msg := newTestMessage(`[{"key":"val"},{"key":"val2"}]`)
	result := aggregator.Process(msg)
	assert.Equal(t, 1, len(result), "Expected one message for complete array")
	assert.Equal(t, []byte(`[{"key":"val"},{"key":"val2"}]`), result[0].GetContent())
}

func TestJSONAggregatorPrefixWithObject(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	msg1 := newTestMessage(`2024-01-01 error details: {`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected no messages for incomplete JSON with prefix")

	msg2 := newTestMessage(`  "error": "something bad",`)
	result = aggregator.Process(msg2)
	assert.Equal(t, 0, len(result))

	msg3 := newTestMessage(`  "code": 500`)
	result = aggregator.Process(msg3)
	assert.Equal(t, 0, len(result))

	msg4 := newTestMessage(`}`)
	result = aggregator.Process(msg4)
	assert.Equal(t, 1, len(result), "Expected one aggregated message")
	assert.Equal(t, []byte(`2024-01-01 error details: {"error":"something bad","code":500}`), result[0].GetContent())
	assert.Contains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag)
}

func TestJSONAggregatorPrefixWithArray(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	msg1 := newTestMessage(`0001-01-01 We have encountered a really bad error: [`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result))

	msg2 := newTestMessage(`  {`)
	result = aggregator.Process(msg2)
	assert.Equal(t, 0, len(result))

	msg3 := newTestMessage(`    "but": "it is",`)
	result = aggregator.Process(msg3)
	assert.Equal(t, 0, len(result))

	msg4 := newTestMessage(`    "pretty": "printed"`)
	result = aggregator.Process(msg4)
	assert.Equal(t, 0, len(result))

	msg5 := newTestMessage(`  },`)
	result = aggregator.Process(msg5)
	assert.Equal(t, 0, len(result))

	msg6 := newTestMessage(`  {`)
	result = aggregator.Process(msg6)
	assert.Equal(t, 0, len(result))

	msg7 := newTestMessage(`    "and in": "array format"`)
	result = aggregator.Process(msg7)
	assert.Equal(t, 0, len(result))

	msg8 := newTestMessage(`  }`)
	result = aggregator.Process(msg8)
	assert.Equal(t, 0, len(result))

	msg9 := newTestMessage(`]`)
	result = aggregator.Process(msg9)
	assert.Equal(t, 1, len(result), "Expected one aggregated message")
	assert.Equal(t, []byte(`0001-01-01 We have encountered a really bad error: [{"but":"it is","pretty":"printed"},{"and in":"array format"}]`), result[0].GetContent())
	assert.Contains(t, result[0].ParsingExtra.Tags, message.AggregatedJSONTag)
}

func TestJSONAggregatorPrefixFalsePositive(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Line ends with [ but subsequent content is not valid JSON
	msg1 := newTestMessage(`Some log message [`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result), "Expected buffering due to trailing [")

	msg2 := newTestMessage(`not valid json content`)
	result = aggregator.Process(msg2)
	assert.Equal(t, 2, len(result), "Expected both messages flushed on invalid")
	assert.Equal(t, []byte(`Some log message [`), result[0].GetContent(), "Original first message preserved")
	assert.Equal(t, []byte(`not valid json content`), result[1].GetContent(), "Original second message preserved")
}

func TestJSONAggregatorNoPrefixDetectionMidStream(t *testing.T) {
	aggregator := NewJSONAggregator(true, 1000)

	// Start with a valid JSON opening
	msg1 := newTestMessage(`{`)
	result := aggregator.Process(msg1)
	assert.Equal(t, 0, len(result))

	// Second message ending with [ should NOT trigger prefix detection
	msg2 := newTestMessage(`"arr": [`)
	result = aggregator.Process(msg2)
	assert.Equal(t, 0, len(result))

	msg3 := newTestMessage(`1, 2, 3`)
	result = aggregator.Process(msg3)
	assert.Equal(t, 0, len(result))

	msg4 := newTestMessage(`]`)
	result = aggregator.Process(msg4)
	assert.Equal(t, 0, len(result))

	msg5 := newTestMessage(`}`)
	result = aggregator.Process(msg5)
	assert.Equal(t, 1, len(result), "Expected one aggregated message")
	assert.Equal(t, []byte(`{"arr":[1,2,3]}`), result[0].GetContent())
}

func TestFindEmbeddedJSONStart(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"trailing brace", "error: {", 7},
		{"trailing bracket", "error: [", 7},
		{"trailing brace with whitespace", "error: {  ", 7},
		{"trailing bracket with whitespace", "error: [  \t\n", 7},
		{"no trailing delimiter", "error: something", -1},
		{"just brace", "{", 0},
		{"just bracket", "[", 0},
		{"empty", "", -1},
		{"only whitespace", "   ", -1},
		{"brace mid-string", "error { more text", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findEmbeddedJSONStart([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}
