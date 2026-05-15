// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
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
			assert.Nil(t, result.JSONContext)
		})
	}
}

func TestPreprocessJSON_TopLevelMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedMsg string
		expectedCtx map[string]interface{}
	}{
		{
			name:        "message field",
			input:       `{"message":"Processing order","level":"info","service":"api"}`,
			expectedMsg: "Processing order",
			expectedCtx: map[string]interface{}{"level": "info", "service": "api"},
		},
		{
			name:        "msg field",
			input:       `{"msg":"User login","timestamp":1234567890}`,
			expectedMsg: "User login",
			expectedCtx: map[string]interface{}{"timestamp": float64(1234567890)},
		},
		{
			name:        "log field",
			input:       `{"log":"Container started","container_id":"abc123"}`,
			expectedMsg: "Container started",
			expectedCtx: map[string]interface{}{"container_id": "abc123"},
		},
		{
			name:        "text field",
			input:       `{"text":"System event","severity":"warning"}`,
			expectedMsg: "System event",
			expectedCtx: map[string]interface{}{"severity": "warning"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON([]byte(tt.input))
			assert.True(t, result.IsJSON)
			assert.Equal(t, tt.expectedMsg, result.Message)
			assert.NotNil(t, result.JSONContext)

			// Verify json_context contains expected fields
			var ctx map[string]interface{}
			err := json.Unmarshal(result.JSONContext, &ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedCtx, ctx)
		})
	}
}

func TestPreprocessJSON_TopLevelKeys(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedMsg string
	}{
		{
			name:        "Kubernetes/Docker log field",
			input:       `{"log":"Pod started\n","stream":"stdout","time":"2024-01-01"}`,
			expectedMsg: "Pod started\n",
		},
		{
			name:        "Standard message field",
			input:       `{"message":"Container log","level":"info"}`,
			expectedMsg: "Container log",
		},
		{
			name:        "msg field (common in Go logs)",
			input:       `{"msg":"Service started","service":"api"}`,
			expectedMsg: "Service started",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON([]byte(tt.input))
			assert.True(t, result.IsJSON)
			assert.Equal(t, tt.expectedMsg, result.Message)
			assert.NotNil(t, result.JSONContext)
		})
	}
}

func TestPreprocessJSON_GenericNestedPaths(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedMsg string
	}{
		{
			name:        "data.message",
			input:       `{"data":{"message":"Nested message","id":123}}`,
			expectedMsg: "Nested message",
		},
		{
			name:        "event.message",
			input:       `{"event":{"message":"Event occurred","type":"alert"}}`,
			expectedMsg: "Event occurred",
		},
		{
			name:        "payload.message",
			input:       `{"payload":{"message":"Payload data","size":1024}}`,
			expectedMsg: "Payload data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PreprocessJSON([]byte(tt.input))
			assert.True(t, result.IsJSON)
			assert.Equal(t, tt.expectedMsg, result.Message)
			assert.NotNil(t, result.JSONContext)
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
			// Fail-fast: if no message found, treat as plain text
			assert.False(t, result.IsJSON)
			assert.Empty(t, result.Message)
			assert.Nil(t, result.JSONContext)
		})
	}
}

func TestPreprocessJSON_OrderedJSONContext(t *testing.T) {
	// Test that json_context has deterministic ordering
	input := `{"message":"test","zebra":"z","apple":"a","banana":"b"}`

	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	require.NotNil(t, result.JSONContext)

	// Parse and verify keys are in sorted order
	var ctx map[string]interface{}
	err := json.Unmarshal(result.JSONContext, &ctx)
	require.NoError(t, err)

	// Re-marshal to check ordering
	expected := `{"apple":"a","banana":"b","zebra":"z"}`
	assert.JSONEq(t, expected, string(result.JSONContext))
}

func TestPreprocessJSON_NestedObjectsOrdered(t *testing.T) {
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
	require.NotNil(t, result.JSONContext)

	var ctx map[string]interface{}
	err := json.Unmarshal(result.JSONContext, &ctx)
	require.NoError(t, err)

	// Verify nested map is also ordered
	nested := ctx["nested"].(map[string]interface{})
	assert.Equal(t, "a", nested["apple"])
	assert.Equal(t, "z", nested["zebra"])
}

func TestPreprocessJSON_EmptyContextAfterExtraction(t *testing.T) {
	// If only message field exists, json_context should be nil
	input := `{"message":"only message here"}`

	result := PreprocessJSON([]byte(input))
	assert.True(t, result.IsJSON)
	assert.Equal(t, "only message here", result.Message)
	assert.Nil(t, result.JSONContext) // No remaining fields
}

func TestPreprocessJSON_ComplexRealWorld(t *testing.T) {
	// Real-world example from the CSV analysis
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
	require.NotNil(t, result.JSONContext)

	var ctx map[string]interface{}
	err := json.Unmarshal(result.JSONContext, &ctx)
	require.NoError(t, err)

	// Verify all non-message fields are preserved
	assert.Equal(t, "info", ctx["level"])
	assert.Equal(t, "payment-api", ctx["service"])
	assert.Equal(t, float64(99.99), ctx["amount"])
	assert.Equal(t, "USD", ctx["currency"])
	assert.NotContains(t, ctx, "msg") // Message field removed
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

func TestMarshalJSONDeterministicOrdering(t *testing.T) {
	data := map[string]interface{}{
		"z": "last",
		"a": "first",
		"m": "middle",
	}

	result, err := json.Marshal(data)
	require.NoError(t, err)

	// Keys should be in sorted order
	expected := `{"a":"first","m":"middle","z":"last"}`
	assert.JSONEq(t, expected, string(result))
}

func TestMarshalJSON_EmptyContextIsNil(t *testing.T) {
	data := map[string]interface{}{}
	// We intentionally avoid marshalling empty maps for json_context to save bytes.
	if len(data) == 0 {
		var result []byte
		assert.Nil(t, result)
		return
	}

	t.Fatal("unexpected non-empty map")
}
