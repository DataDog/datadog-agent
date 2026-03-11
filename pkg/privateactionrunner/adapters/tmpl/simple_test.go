// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tmpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		shouldError bool
	}{
		{
			name:        "plain text",
			template:    "Hello World",
			shouldError: false,
		},
		{
			name:        "single expression",
			template:    "{{ name }}",
			shouldError: false,
		},
		{
			name:        "nested path expression",
			template:    "{{ user.name }}",
			shouldError: false,
		},
		{
			name:        "multiple expressions",
			template:    "Hello {{ firstName }} {{ lastName }}",
			shouldError: false,
		},
		{
			name:        "expression with bracket notation",
			template:    "{{ [my key] }}",
			shouldError: false,
		},
		{
			name:        "empty template",
			template:    "",
			shouldError: false,
		},
		{
			name:        "invalid - parent path",
			template:    "{{ .. }}",
			shouldError: true,
		},
		{
			name:        "invalid - slash separator",
			template:    "{{ path/to }}",
			shouldError: true,
		},
		{
			name:        "invalid - empty expression",
			template:    "{{ }}",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := Parse(tt.template)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tmpl)
			}
		})
	}
}

func TestRender(t *testing.T) {
	tests := []struct {
		name     string
		template string
		input    interface{}
		expected string
	}{
		{
			name:     "plain text",
			template: "Hello World",
			input:    nil,
			expected: "Hello World",
		},
		{
			name:     "simple variable",
			template: "Hello {{ name }}",
			input:    map[string]interface{}{"name": "Alice"},
			expected: "Hello Alice",
		},
		{
			name:     "nested path",
			template: "{{ user.name }}",
			input:    map[string]interface{}{"user": map[string]interface{}{"name": "Bob"}},
			expected: "Bob",
		},
		{
			name:     "array index with bracket notation",
			template: "{{ items.[0] }}",
			input:    map[string]interface{}{"items": []interface{}{"first", "second"}},
			expected: "first",
		},
		{
			name:     "missing value returns empty",
			template: "{{ missing }}",
			input:    map[string]interface{}{},
			expected: "",
		},
		{
			name:     "integer value",
			template: "Count: {{ count }}",
			input:    map[string]interface{}{"count": 42},
			expected: "Count: 42",
		},
		{
			name:     "float value",
			template: "Price: {{ price }}",
			input:    map[string]interface{}{"price": 19.99},
			expected: "Price: 19.99",
		},
		{
			name:     "boolean value",
			template: "Active: {{ active }}",
			input:    map[string]interface{}{"active": true},
			expected: "Active: true",
		},
		{
			name:     "map value renders as JSON",
			template: "{{ data }}",
			input:    map[string]interface{}{"data": map[string]interface{}{"key": "value"}},
			expected: `{"key":"value"}`,
		},
		{
			name:     "slice value renders as JSON",
			template: "{{ items }}",
			input:    map[string]interface{}{"items": []interface{}{"a", "b"}},
			expected: `["a","b"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Render(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseAndRender(t *testing.T) {
	input := map[string]interface{}{
		"greeting": "Hello",
		"name":     "World",
	}

	result, err := ParseAndRender("{{ greeting }} {{ name }}!", input)
	require.NoError(t, err)
	assert.Equal(t, "Hello World!", result)
}

func TestEvaluatePath(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		path        string
		expected    interface{}
		shouldError bool
	}{
		{
			name:     "simple key",
			input:    map[string]interface{}{"name": "Alice"},
			path:     "name",
			expected: "Alice",
		},
		{
			name:     "nested key",
			input:    map[string]interface{}{"user": map[string]interface{}{"email": "alice@example.com"}},
			path:     "user.email",
			expected: "alice@example.com",
		},
		{
			name:     "array index with bracket notation",
			input:    map[string]interface{}{"items": []interface{}{"a", "b", "c"}},
			path:     "items.[1]",
			expected: "b",
		},
		{
			name:        "invalid path expression",
			input:       map[string]interface{}{},
			path:        "invalid/path",
			shouldError: true,
		},
		{
			name:        "path not found",
			input:       map[string]interface{}{},
			path:        "nonexistent",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := EvaluatePath(tt.input, tt.path)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMustEvaluatePath(t *testing.T) {
	input := map[string]interface{}{"key": "value"}

	// Should succeed
	result := MustEvaluatePath(input, "key")
	assert.Equal(t, "value", result)

	// Should panic on error
	assert.Panics(t, func() {
		MustEvaluatePath(input, "nonexistent")
	})
}

func TestPreserveExpressionsWithPathRoots(t *testing.T) {
	input := map[string]interface{}{
		"local": "localValue",
	}

	// Parse with preservation option for "secrets"
	tmpl, err := Parse("{{ local }} {{ secrets.key }}", PreserveExpressionsWithPathRoots("secrets"))
	require.NoError(t, err)

	result, err := tmpl.Render(input)
	require.NoError(t, err)

	// "local" should be replaced, "secrets.key" should be preserved
	assert.Equal(t, "localValue {{ secrets.key }}", result)
}

func TestErrPathNotFound(t *testing.T) {
	err := ErrPathNotFound{
		FullyQualifiedPath: "user.name",
		Context:            map[string]interface{}{},
	}

	assert.Contains(t, err.Error(), "user.name")
	assert.Contains(t, err.Error(), "not found")
}

func TestParseError(t *testing.T) {
	err := ParseError{
		Description: "unexpected token",
		Pos:         5,
	}

	assert.Contains(t, err.Error(), "unexpected token")
	assert.Contains(t, err.Error(), "pos 5")
}

func TestStringifyTypes(t *testing.T) {
	tests := []struct {
		name     string
		template string
		input    interface{}
		expected string
	}{
		{
			name:     "int",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": int(42)},
			expected: "42",
		},
		{
			name:     "int8",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": int8(8)},
			expected: "8",
		},
		{
			name:     "int16",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": int16(16)},
			expected: "16",
		},
		{
			name:     "int32",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": int32(32)},
			expected: "32",
		},
		{
			name:     "int64",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": int64(64)},
			expected: "64",
		},
		{
			name:     "uint",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": uint(42)},
			expected: "42",
		},
		{
			name:     "uint8",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": uint8(8)},
			expected: "8",
		},
		{
			name:     "uint16",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": uint16(16)},
			expected: "16",
		},
		{
			name:     "uint32",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": uint32(32)},
			expected: "32",
		},
		{
			name:     "uint64",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": uint64(64)},
			expected: "64",
		},
		{
			name:     "float32",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": float32(3.14)},
			expected: "3.140000104904175",
		},
		{
			name:     "float64",
			template: "{{ val }}",
			input:    map[string]interface{}{"val": float64(3.14159)},
			expected: "3.14159",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := Parse(tt.template)
			require.NoError(t, err)

			result, err := tmpl.Render(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStructFieldAccess(t *testing.T) {
	type Person struct {
		Name  string
		Age   int
		Email string
	}

	input := Person{
		Name:  "Alice",
		Age:   30,
		Email: "alice@example.com",
	}

	tmpl, err := Parse("{{ name }}: {{ age }}")
	require.NoError(t, err)

	result, err := tmpl.Render(input)
	require.NoError(t, err)
	assert.Equal(t, "Alice: 30", result)
}

func TestPointerDereference(t *testing.T) {
	type Data struct {
		Value string
	}

	data := &Data{Value: "test"}
	input := map[string]interface{}{"data": data}

	tmpl, err := Parse("{{ data.value }}")
	require.NoError(t, err)

	result, err := tmpl.Render(input)
	require.NoError(t, err)
	assert.Equal(t, "test", result)
}

func TestNilValue(t *testing.T) {
	input := map[string]interface{}{"value": nil}

	tmpl, err := Parse("{{ value }}")
	require.NoError(t, err)

	result, err := tmpl.Render(input)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestBracketNotation(t *testing.T) {
	input := map[string]interface{}{
		"my key with spaces": "value",
	}

	tmpl, err := Parse("{{ [my key with spaces] }}")
	require.NoError(t, err)

	result, err := tmpl.Render(input)
	require.NoError(t, err)
	assert.Equal(t, "value", result)
}

func TestSpecialCharactersInJSON(t *testing.T) {
	input := map[string]interface{}{
		"data": map[string]interface{}{
			"html":    "<script>alert('xss')</script>",
			"special": "a & b",
		},
	}

	tmpl, err := Parse("{{ data }}")
	require.NoError(t, err)

	result, err := tmpl.Render(input)
	require.NoError(t, err)
	// Should not HTML-escape special characters
	assert.Contains(t, result, "<script>")
	assert.Contains(t, result, "&")
}
