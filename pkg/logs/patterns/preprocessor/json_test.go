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
	result := PreprocessJSON([]byte("plain text log"))
	assert.False(t, result.IsJSON)
	assert.Empty(t, result.Message)
}

func TestPreprocessJSON_NoMessageField(t *testing.T) {
	result := PreprocessJSON([]byte(`{"level":"info","service":"api"}`))
	assert.False(t, result.IsJSON)
}

func TestPreprocessJSON_MessageOnly(t *testing.T) {
	result := PreprocessJSON([]byte(`{"message":"hello"}`))
	assert.True(t, result.IsJSON)
	assert.Equal(t, "hello", result.Message)
	assert.Equal(t, "message", result.MessageKey)
	assert.Empty(t, result.JSONContextSchema)
	assert.Empty(t, result.JSONContextValues)
}

func TestPreprocessJSON_MessageWithContext(t *testing.T) {
	result := PreprocessJSON([]byte(`{"message":"Processing","level":"info","pid":1234}`))
	require.True(t, result.IsJSON)
	assert.Equal(t, "Processing", result.Message)
	assert.Equal(t, "level,pid", result.JSONContextSchema)
	require.Len(t, result.JSONContextValues, 2)
	assert.Equal(t, "info", result.JSONContextValues[0])
	// pid must be json.Number (not float64 or string) to preserve integer precision.
	// Failing here means UseNumber is not configured, causing float64 coercion.
	pidNum, ok := result.JSONContextValues[1].(interface{ String() string })
	require.True(t, ok, "pid must be json.Number, not float64 or string")
	assert.Equal(t, "1234", pidNum.String())
}

func TestPreprocessJSON_LargeIntPreserved(t *testing.T) {
	// Integers exceeding float64 precision (>2^53) must not become scientific notation.
	// UseNumber preserves the exact string representation.
	input := `{"message":"evt","eventHash":163631252358535540000}`
	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	require.Len(t, result.JSONContextValues, 1)
	// Value should be a json.Number preserving original digits
	numStr, ok := result.JSONContextValues[0].(interface{ String() string })
	require.True(t, ok, "large int must be json.Number, not float64")
	assert.Equal(t, "163631252358535540000", numStr.String())
}

func TestPreprocessJSON_MsgKey(t *testing.T) {
	result := PreprocessJSON([]byte(`{"msg":"User login","timestamp":1234567890}`))
	require.True(t, result.IsJSON)
	assert.Equal(t, "User login", result.Message)
	assert.Equal(t, "msg", result.MessageKey)
}

func TestPreprocessJSON_NestedPath(t *testing.T) {
	result := PreprocessJSON([]byte(`{"data":{"message":"Nested","id":123}}`))
	require.True(t, result.IsJSON)
	assert.Equal(t, "Nested", result.Message)
	assert.Equal(t, "data.message", result.MessageKey)
}

func TestPreprocessJSON_NullValue(t *testing.T) {
	result := PreprocessJSON([]byte(`{"message":"test","extra":null}`))
	require.True(t, result.IsJSON)
	assert.Equal(t, "extra", result.JSONContextSchema)
	assert.Equal(t, []interface{}{nil}, result.JSONContextValues)
}

func TestPreprocessJSON_BoolValue(t *testing.T) {
	result := PreprocessJSON([]byte(`{"message":"test","active":true}`))
	require.True(t, result.IsJSON)
	assert.Equal(t, "active", result.JSONContextSchema)
	assert.Equal(t, []interface{}{true}, result.JSONContextValues)
}

// --- Production mismatch regression tests ---

func TestPreprocessJSON_StagingMismatch_MessageWithTabs(t *testing.T) {
	// journald log where message field contains \t separators
	input := `{"message":"Importing\telapsed: 0.4 s\ttotal:   0.0 B\t(0.0 B/s)","journald":{"_COMM":"ctr"}}`
	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	assert.Equal(t, "Importing\telapsed: 0.4 s\ttotal:   0.0 B\t(0.0 B/s)", result.Message)
}

func TestPreprocessJSON_NestedObjectPreserved(t *testing.T) {
	// Nested objects are kept as decoded map values ([]interface{} type)
	input := `{"message":"test","metadata":{"region":"us-east-1","zone":"a"}}`
	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	assert.Equal(t, "metadata", result.JSONContextSchema)
	require.Len(t, result.JSONContextValues, 1)
	nested, ok := result.JSONContextValues[0].(map[string]interface{})
	require.True(t, ok, "nested object must be map[string]interface{}")
	assert.Equal(t, "us-east-1", nested["region"])
}

func TestPreprocessJSON_IsJSONObject(t *testing.T) {
	assert.True(t, IsJSONObject([]byte(`{"key":"val"}`)))
	assert.True(t, IsJSONObject([]byte(`  {"key":"val"}`)))  // leading whitespace
	assert.False(t, IsJSONObject([]byte(`["array"]`)))
	assert.False(t, IsJSONObject([]byte(`plain text`)))
	assert.False(t, IsJSONObject([]byte(``)))
}

func TestPreprocessJSON_StagingMismatch_NewlineInMessage(t *testing.T) {
	// \n in a JSON string value must be returned verbatim in Message.
	// PreprocessJSON does not strip whitespace from extracted values — stripping
	// is the responsibility of sanitizeForTemplateInto downstream.
	// This test guards against any accidental trimming in the extraction layer.
	input := `{"message":"hint_type: DELETE\nlimit: 5000\n","logger":"cell"}`
	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	// JSON \n in a string → real newline char after parsing
	assert.Equal(t, "hint_type: DELETE\nlimit: 5000\n", result.Message,
		"newlines in JSON string values must survive extraction unchanged")
	// The fix for stripping in the final reconstructed log is in sanitizeForTemplateInto,
	// not here — this test confirms the extraction layer does not interfere.
}

func TestPreprocessJSON_StagingMismatch_TrailingSpaceInMessage(t *testing.T) {
	// Real mismatch (2026-04-23 staging): trailing space in msg field was stripped.
	// HTTP:  "Checking error " (with trailing space)
	// gRPC:  "Checking error"  (no trailing space)
	input := `{"msg":"Checking error ","level":"INFO"}`
	result := PreprocessJSON([]byte(input))
	require.True(t, result.IsJSON)
	assert.Equal(t, "Checking error ", result.Message,
		"trailing space in message field must be returned verbatim")
}
