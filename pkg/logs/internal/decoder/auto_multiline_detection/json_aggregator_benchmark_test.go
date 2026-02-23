// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"testing"
)

func BenchmarkJSONAggregator_SingleLineJSON(b *testing.B) {
	aggregator := NewJSONAggregator(true, 10000)
	jsonContent := `{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"test message","user_id":12345,"request_id":"abc-123","duration_ms":150}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := newTestMessage(jsonContent)
		result := aggregator.Process(msg)
		if len(result) != 1 {
			b.Fatal("Expected 1 message")
		}
	}
}

// BenchmarkJSONAggregator_InvalidSingleLineJSON tests the case where isSingleLineJSON() returns true
// (balanced braces) but json.Valid() returns false (invalid JSON syntax).
// This should fall through to the full decoder path.
func BenchmarkJSONAggregator_InvalidSingleLineJSON(b *testing.B) {
	aggregator := NewJSONAggregator(true, 10000)
	// Invalid JSON: missing quotes around some keys, but has balanced braces
	jsonContent := `{"timestamp":"2024-01-01T00:00:00Z",level:info,message:"test message","user_id":12345,"request_id":"abc-123","duration_ms":150}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := newTestMessage(jsonContent)
		result := aggregator.Process(msg)
		if len(result) != 1 {
			b.Fatal("Expected 1 message")
		}
	}
}

func BenchmarkJSONAggregator_MultilineJSON(b *testing.B) {
	aggregator := NewJSONAggregator(true, 10000)
	part1 := `{"timestamp":"2024-01-01T00:00:00Z","level":"info",`
	part2 := `"message":"test message","user_id":12345,`
	part3 := `"request_id":"abc-123","duration_ms":150}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg1 := newTestMessage(part1)
		aggregator.Process(msg1)
		msg2 := newTestMessage(part2)
		aggregator.Process(msg2)
		msg3 := newTestMessage(part3)
		result := aggregator.Process(msg3)
		if len(result) != 1 {
			b.Fatal("Expected 1 message")
		}
	}
}

func BenchmarkJSONAggregator_ComplexNestedJSON(b *testing.B) {
	aggregator := NewJSONAggregator(true, 10000)
	jsonContent := `{"timestamp":"2024-01-01T00:00:00Z","user":{"id":12345,"name":"test","email":"test@example.com"},"request":{"method":"GET","path":"/api/test","headers":{"content-type":"application/json"}},"response":{"status":200,"body":{"success":true,"data":["item1","item2","item3"]}}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := newTestMessage(jsonContent)
		result := aggregator.Process(msg)
		if len(result) != 1 {
			b.Fatal("Expected 1 message")
		}
	}
}

// BenchmarkJSONAggregator_UnbalancedBraces tests incomplete JSON with unbalanced braces
// This skips the fast-path entirely (isSingleLineJSON returns false)
func BenchmarkJSONAggregator_UnbalancedBraces(b *testing.B) {
	aggregator := NewJSONAggregator(true, 10000)
	// Incomplete JSON - missing closing brace
	jsonContent := `{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"incomplete`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := newTestMessage(jsonContent)
		result := aggregator.Process(msg)
		// Should return empty (incomplete JSON gets buffered)
		if len(result) != 0 {
			b.Fatal("Expected 0 messages for incomplete JSON")
		}
		// Flush to reset for next iteration
		aggregator.Flush()
	}
}
