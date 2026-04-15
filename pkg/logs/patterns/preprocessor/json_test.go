// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreprocessJSON_NotJSON(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "plain text",
			content: []byte("This is a plain log message"),
		},
		{
			name:    "empty",
			content: []byte(""),
		},
		{
			name:    "invalid JSON",
			content: []byte("{invalid json}"),
		},
		{
			name:    "starts with text",
			content: []byte("2024-01-01 ERROR something"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON(tt.content)
			assert.False(t, result.IsJSON)
			assert.Empty(t, result.Message)
			assert.Empty(t, result.JSONContextSchema)
			assert.Nil(t, result.JSONContextValues)
		})
	}
}

func TestPreprocessJSON_TopLevelMessage(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedMsg    string
		expectedKey    string
		expectedSchema string
		expectedValues []string
	}{
		{
			name:           "message field",
			input:          `{"message":"Processing order","level":"info","service":"api"}`,
			expectedMsg:    "Processing order",
			expectedKey:    "message",
			expectedSchema: "level,service",
			expectedValues: []string{"info", "api"},
		},
		{
			name:           "msg field",
			input:          `{"msg":"User login","timestamp":1234567890}`,
			expectedMsg:    "User login",
			expectedKey:    "msg",
			expectedSchema: "timestamp",
			expectedValues: []string{"1.23456789e+09"},
		},
		{
			name:           "log field",
			input:          `{"log":"Container started","container_id":"abc123"}`,
			expectedMsg:    "Container started",
			expectedKey:    "log",
			expectedSchema: "container_id",
			expectedValues: []string{"abc123"},
		},
		{
			name:           "text field",
			input:          `{"text":"System event","severity":"warning"}`,
			expectedMsg:    "System event",
			expectedKey:    "text",
			expectedSchema: "severity",
			expectedValues: []string{"warning"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON([]byte(tt.input))
			assert.True(t, result.IsJSON)
			assert.Equal(t, tt.expectedMsg, result.Message)
			assert.Equal(t, tt.expectedKey, result.MessageKey)
			assert.Equal(t, tt.expectedSchema, result.JSONContextSchema)
			assert.Equal(t, tt.expectedValues, result.JSONContextValues)
		})
	}
}

func TestPreprocessJSON_TopLevelKeys(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedMsg    string
		expectedSchema string
	}{
		{
			name:           "Kubernetes/Docker log field",
			input:          `{"log":"Pod started\n","stream":"stdout","time":"2024-01-01"}`,
			expectedMsg:    "Pod started\n",
			expectedSchema: "stream,time",
		},
		{
			name:           "Standard message field",
			input:          `{"message":"Container log","level":"info"}`,
			expectedMsg:    "Container log",
			expectedSchema: "level",
		},
		{
			name:           "msg field (common in Go logs)",
			input:          `{"msg":"Service started","service":"api"}`,
			expectedMsg:    "Service started",
			expectedSchema: "service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON([]byte(tt.input))
			assert.True(t, result.IsJSON)
			assert.Equal(t, tt.expectedMsg, result.Message)
			assert.Equal(t, tt.expectedSchema, result.JSONContextSchema)
			assert.NotEmpty(t, result.JSONContextValues)
		})
	}
}

func TestPreprocessJSON_GenericNestedPaths(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedMsg    string
		expectedSchema string
	}{
		{
			name:           "data.message",
			input:          `{"data":{"message":"Nested message","id":123}}`,
			expectedMsg:    "Nested message",
			expectedSchema: "data",
		},
		{
			name:           "event.message",
			input:          `{"event":{"message":"Event occurred","type":"alert"}}`,
			expectedMsg:    "Event occurred",
			expectedSchema: "event",
		},
		{
			name:           "payload.message",
			input:          `{"payload":{"message":"Payload data","size":1024}}`,
			expectedMsg:    "Payload data",
			expectedSchema: "payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON([]byte(tt.input))
			assert.True(t, result.IsJSON)
			assert.Equal(t, tt.expectedMsg, result.Message)
			assert.Equal(t, tt.expectedSchema, result.JSONContextSchema)
			assert.NotEmpty(t, result.JSONContextValues)
		})
	}
}

func TestPreprocessJSON_NoMessageFound(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "no message field",
			input: `{"level":"info","service":"api","timestamp":1234567890}`,
		},
		{
			name:  "empty message",
			input: `{"message":"","level":"info"}`,
		},
		{
			name:  "message is not string",
			input: `{"message":123,"level":"info"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON([]byte(tt.input))
			assert.False(t, result.IsJSON)
			assert.Empty(t, result.Message)
			assert.Empty(t, result.JSONContextSchema)
			assert.Nil(t, result.JSONContextValues)
		})
	}
}

func TestPreprocessJSON_OrderedJSONContext(t *testing.T) {
	// Test that json_context has deterministic ordering
	input := `{"message":"test","zebra":"z","apple":"a","banana":"b"}`

	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	assert.Equal(t, "apple,banana,zebra", result.JSONContextSchema)
	assert.Equal(t, []string{"a", "b", "z"}, result.JSONContextValues)
}

func TestPreprocessJSON_NestedObjectsAsValues(t *testing.T) {
	input := `{
		"message":"test",
		"nested": {
			"zebra":"z",
			"apple":"a"
		},
		"array": [1, 2, 3]
	}`

	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	assert.Equal(t, "array,nested", result.JSONContextSchema)
	require.Len(t, result.JSONContextValues, 2)
	assert.Equal(t, "[1,2,3]", result.JSONContextValues[0])
	assert.Equal(t, `{"apple":"a","zebra":"z"}`, result.JSONContextValues[1])
}

func TestPreprocessJSON_EmptyContextAfterExtraction(t *testing.T) {
	input := `{"message":"only message here"}`

	result := PreprocessJSON([]byte(input))
	assert.True(t, result.IsJSON)
	assert.Equal(t, "only message here", result.Message)
	assert.Equal(t, "message", result.MessageKey)
	assert.Empty(t, result.JSONContextSchema)
	assert.Nil(t, result.JSONContextValues)
}

func TestPreprocessJSON_ComplexRealWorld(t *testing.T) {
	input := `{
		"level":"info",
		"msg":"Processing payment",
		"service":"payment-api",
		"timestamp":"2024-01-01T12:00:00Z",
		"user_id":"user123",
		"amount":99.99,
		"currency":"USD"
	}`

	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	assert.Equal(t, "Processing payment", result.Message)
	assert.Equal(t, "msg", result.MessageKey)
	assert.Equal(t, "amount,currency,level,service,timestamp,user_id", result.JSONContextSchema)
	require.Len(t, result.JSONContextValues, 6)
	assert.Equal(t, "99.99", result.JSONContextValues[0])
	assert.Equal(t, "USD", result.JSONContextValues[1])
	assert.Equal(t, "info", result.JSONContextValues[2])
	assert.Equal(t, "payment-api", result.JSONContextValues[3])
	assert.Equal(t, "2024-01-01T12:00:00Z", result.JSONContextValues[4])
	assert.Equal(t, "user123", result.JSONContextValues[5])
}

func TestGetValueByPath(t *testing.T) {
	data := map[string]interface{}{
		"top": "value",
		"nested": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": "deep value",
			},
		},
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"top", "value"},
		{"nested.level2.level3", "deep value"},
		{"nonexistent", ""},
		{"nested.nonexistent", ""},
		{"nested.level2.nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := getValueByPath(data, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRemoveFieldByPath(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		path     string
		expected map[string]interface{}
	}{
		{
			name:     "top level",
			data:     map[string]interface{}{"a": "1", "b": "2"},
			path:     "a",
			expected: map[string]interface{}{"b": "2"},
		},
		{
			name: "nested",
			data: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
					"keep":  "this",
				},
			},
			path: "outer.inner",
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"keep": "this",
				},
			},
		},
		{
			name:     "nonexistent path",
			data:     map[string]interface{}{"a": "1"},
			path:     "nonexistent",
			expected: map[string]interface{}{"a": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removeFieldByPath(tt.data, tt.path)
			assert.Equal(t, tt.expected, tt.data)
		})
	}
}

func TestExtractSchemaAndValues(t *testing.T) {
	data := map[string]interface{}{
		"z": "last",
		"a": "first",
		"m": "middle",
	}

	schema, values := extractSchemaAndValues(data)
	assert.Equal(t, "a,m,z", schema)
	assert.Equal(t, []string{"first", "middle", "last"}, values)
}

func TestExtractSchemaAndValues_MixedTypes(t *testing.T) {
	data := map[string]interface{}{
		"count":  float64(42),
		"flag":   true,
		"name":   "test",
		"nested": map[string]interface{}{"key": "val"},
		"empty":  nil,
	}

	schema, values := extractSchemaAndValues(data)
	assert.Equal(t, "count,empty,flag,name,nested", schema)
	assert.Equal(t, []string{"42", "", "true", "test", `{"key":"val"}`}, values)
}
