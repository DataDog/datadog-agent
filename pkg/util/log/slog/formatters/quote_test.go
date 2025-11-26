// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", `"hello"`},
		{"hello world", `"hello world"`},
		{"", `""`},
		{"hello\nworld", `"hello\nworld"`},
		{"hello\tworld", `"hello\tworld"`},
		{`hello"world`, `"hello\"world"`},
		{`hello\world`, `"hello\\world"`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Quote(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQuoteSpecialCharacters(t *testing.T) {
	// Test various special characters
	result := Quote("line1\nline2\ttab\rcarriage")
	assert.Contains(t, result, `\n`)
	assert.Contains(t, result, `\t`)
	assert.Contains(t, result, `\r`)
}

func TestQuoteUnicode(t *testing.T) {
	result := Quote("hello 世界")
	assert.Contains(t, result, "hello")
	// Unicode characters should be preserved or escaped
	assert.NotEmpty(t, result)
}

func TestQuoteEmptyString(t *testing.T) {
	result := Quote("")
	assert.Equal(t, `""`, result)
}
