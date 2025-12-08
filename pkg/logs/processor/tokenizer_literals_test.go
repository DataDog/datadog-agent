// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
	"github.com/stretchr/testify/assert"
)

func TestTokenizerWithLiterals(t *testing.T) {
	tokenizer := automultilinedetection.NewTokenizerWithLiterals(1000)

	testCases := []struct {
		name     string
		input    string
		expected []tokens.Token
	}{
		{
			name:  "simple string",
			input: "app_key",
			expected: []tokens.Token{
				tokens.NewToken(tokens.C3, "app"),
				tokens.NewToken(tokens.Underscore, "_"),
				tokens.NewToken(tokens.C3, "key"),
			},
		},
		{
			name:  "longer string",
			input: "application_key",
			expected: []tokens.Token{
				tokens.NewToken(tokens.C10, "application"), // "application" is 11 chars but maxRun is 10
				tokens.NewToken(tokens.C1, "n"),
				tokens.NewToken(tokens.Underscore, "_"),
				tokens.NewToken(tokens.C3, "key"),
			},
		},
		{
			name:  "SSN pattern",
			input: "123-45-6789",
			expected: []tokens.Token{
				tokens.NewToken(tokens.D3, "123"),
				tokens.NewToken(tokens.Dash, "-"),
				tokens.NewToken(tokens.D2, "45"),
				tokens.NewToken(tokens.Dash, "-"),
				tokens.NewToken(tokens.D4, "6789"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			toks, _ := tokenizer.Tokenize([]byte(tc.input))

			assert.Equal(t, len(tc.expected), len(toks), "Token count mismatch")

			for i, expected := range tc.expected {
				assert.Equal(t, expected.Kind, toks[i].Kind, "Token kind mismatch at position %d", i)
				assert.Equal(t, expected.Lit, toks[i].Lit, "Token literal mismatch at position %d", i)
			}
		})
	}
}

func TestRedactorWithLiterals(t *testing.T) {
	config := ProcessingRuleApplicatorConfig{}
	redactor := NewRedactor(config)
	tokenizer := automultilinedetection.NewTokenizerWithLiterals(1000)

	testCases := []struct {
		name     string
		input    string
		expected string
		rules    []string
	}{
		{
			name:     "app_key redaction",
			input:    "config: app_key=a1b2c3d4e5f67890a1b2c3d4e5f67890ab",
			expected: "config: [APP_KEY_REDACTED]",
			rules:    []string{"auto_redact_app_key"},
		},
		{
			name:     "application_key redaction",
			input:    "config: application_key=a1b2c3d4e5f67890a1b2c3d4e5f67890ab",
			expected: "config: [APP_KEY_REDACTED]",
			rules:    []string{"auto_redact_application_key"},
		},
		{
			name:     "SSN redaction",
			input:    "SSN: 123-45-6789",
			expected: "SSN: [SSN_REDACTED]",
			rules:    []string{"auto_redact_ssn"},
		},
		{
			name:     "SSN numbers only",
			input:    "SSN: 123456789",
			expected: "SSN: [SSN_REDACTED]",
			rules:    []string{"auto_redact_ssn_numbers_only"},
		},
		{
			name:     "no match - different key name",
			input:    "config: api_key=a1b2c3d4e5f67890a1b2c3d4e5f67890ab",
			expected: "config: api_key=a1b2c3d4e5f67890a1b2c3d4e5f67890ab",
			rules:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, matchedRules := redactor.Apply([]byte(tc.input), tokenizer)

			assert.Equal(t, tc.expected, string(result), "Redaction result mismatch")
			assert.Equal(t, tc.rules, matchedRules, "Matched rules mismatch")
		})
	}
}

func TestTokenEquals(t *testing.T) {
	testCases := []struct {
		name     string
		token    tokens.Token
		pattern  tokens.Token
		expected bool
	}{
		{
			name:     "exact match with literal",
			token:    tokens.NewToken(tokens.C3, "app"),
			pattern:  tokens.NewToken(tokens.C3, "app"),
			expected: true,
		},
		{
			name:     "kind match, no literal in pattern",
			token:    tokens.NewToken(tokens.C3, "foo"),
			pattern:  tokens.NewSimpleToken(tokens.C3),
			expected: true,
		},
		{
			name:     "kind match but literal mismatch",
			token:    tokens.NewToken(tokens.C3, "foo"),
			pattern:  tokens.NewToken(tokens.C3, "bar"),
			expected: false,
		},
		{
			name:     "kind mismatch",
			token:    tokens.NewToken(tokens.C3, "app"),
			pattern:  tokens.NewToken(tokens.D3, "123"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.token.Equals(tc.pattern)
			assert.Equal(t, tc.expected, result)
		})
	}
}
