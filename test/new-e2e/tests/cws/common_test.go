// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"testing"
)

func TestEscapeSQLString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "hostname",
			expected: "hostname",
		},
		{
			name:     "single quote",
			input:    "host'name",
			expected: "host''name",
		},
		{
			name:     "multiple single quotes",
			input:    "host'name'test",
			expected: "host''name''test",
		},
		{
			name:     "sql injection attempt - basic",
			input:    "'; DROP TABLE users;--",
			expected: "''; DROP TABLE users;--",
		},
		{
			name:     "sql injection attempt - complex",
			input:    "admin' OR '1'='1",
			expected: "admin'' OR ''1''=''1",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only single quotes",
			input:    "'''",
			expected: "''''''",
		},
		{
			name:     "mixed characters with quotes",
			input:    "host-name_123'test",
			expected: "host-name_123''test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeSQLString(tt.input)
			if result != tt.expected {
				t.Errorf("escapeSQLString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapeSQLStringPreventsSQLInjection(t *testing.T) {
	// Test that common SQL injection patterns are properly escaped
	dangerousInputs := []string{
		"'; DROP TABLE host;--",
		"' OR 1=1--",
		"' UNION SELECT * FROM users--",
		"admin'; DELETE FROM users WHERE '1'='1",
		"'; INSERT INTO logs VALUES ('hacked');--",
	}

	for _, dangerous := range dangerousInputs {
		t.Run("dangerous_input_"+dangerous[:10], func(t *testing.T) {
			escaped := escapeSQLString(dangerous)

			// Verify that the escaped string doesn't start with a quote
			// that would allow breaking out of the original query context
			if len(escaped) > 0 && escaped[0] == '\'' {
				// If it starts with a quote, the next char should also be a quote (escaped)
				if len(escaped) < 2 || escaped[1] != '\'' {
					t.Errorf("escapeSQLString(%q) = %q, dangerous unescaped quote at start", dangerous, escaped)
				}
			}

			// Verify that single quotes are properly doubled
			singleQuoteCount := 0
			doubleQuoteCount := 0
			for i, char := range escaped {
				if char == '\'' {
					if i+1 < len(escaped) && escaped[i+1] == '\'' {
						doubleQuoteCount++
					} else {
						singleQuoteCount++
					}
				}
			}

			if singleQuoteCount > 0 {
				t.Errorf("escapeSQLString(%q) = %q, contains unescaped single quotes", dangerous, escaped)
			}
		})
	}
}
