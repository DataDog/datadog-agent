//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rtokenizer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/clustering"
	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestRustTokenizer_ClusteringIntegration tests the full integration:
// Rust tokenizer → Agent TokenList → Clustering
// This validates that all token types work correctly in the clustering pipeline.
func TestRustTokenizer_ClusteringIntegration(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	tests := []struct {
		name                 string
		logs                 []string
		expectedTokens       []token.TokenType // token types we expect to see
		description          string
		expectedPatternCount int
	}{
		{
			name: "Basic word tokens",
			logs: []string{
				"Service started",
				"Service stopped",
			},
			expectedTokens: []token.TokenType{
				token.TokenWord,
				token.TokenWhitespace,
			},
			description:          "Basic word and whitespace tokens should cluster together",
			expectedPatternCount: 1,
		},
		{
			name: "HTTP logs with different methods",
			logs: []string{
				"GET /api/users 200",
				"POST /api/users 201",
				"PUT /api/users 200",
			},
			expectedTokens: []token.TokenType{
				token.TokenHTTPMethod,
				token.TokenWhitespace,
				token.TokenAbsolutePath,
				token.TokenWhitespace,
				token.TokenHTTPStatus,
			},
			description:          "HTTP logs with different methods should cluster by path",
			expectedPatternCount: 1,
		},
		{
			name: "Logs with IP addresses",
			logs: []string{
				"Connection from 192.168.1.1",
				"Connection from 10.0.0.1",
				"Connection from 172.16.0.1",
			},
			expectedTokens: []token.TokenType{
				token.TokenWord,
				token.TokenWhitespace,
				token.TokenWord,
				token.TokenWhitespace,
				token.TokenIPv4,
			},
			description:          "Logs with different IP addresses should cluster together",
			expectedPatternCount: 1,
		},
		{
			name: "Logs with email addresses",
			logs: []string{
				"User admin@example.com logged in",
				"User user@test.com logged in",
			},
			expectedTokens: []token.TokenType{
				token.TokenWord,
				token.TokenWhitespace,
				token.TokenEmail,
				token.TokenWhitespace,
				token.TokenWord,
			},
			description:          "Logs with different email addresses should cluster together",
			expectedPatternCount: 1,
		},
		{
			name: "Logs with URIs",
			logs: []string{
				"Request to https://example.com/api",
				"Request to https://test.com/api",
			},
			expectedTokens: []token.TokenType{
				token.TokenWord,
				token.TokenWhitespace,
				token.TokenWord,
				token.TokenWhitespace,
				token.TokenURI,
			},
			description:          "Logs with different URIs should cluster together",
			expectedPatternCount: 1,
		},
		{
			name: "Logs with severity levels",
			logs: []string{
				"ERROR Database connection failed",
				"WARN Database connection slow",
				"INFO Database connection established",
			},
			expectedTokens: []token.TokenType{
				token.TokenSeverityLevel,
				token.TokenWhitespace,
				token.TokenWord,
			},
			description:          "Logs with different severity levels should cluster by message",
			expectedPatternCount: 1,
		},
		{
			name: "Logs with dates and times",
			logs: []string{
				"2024-01-15 10:30:00 Server started",
				"2024-01-15 11:00:00 Server started",
			},
			expectedTokens: []token.TokenType{
				token.TokenLocalDateTime,
				token.TokenWhitespace,
				token.TokenWord,
			},
			description:          "Logs with different timestamps should cluster together",
			expectedPatternCount: 1,
		},
		{
			name: "Logs with numeric values",
			logs: []string{
				"Processing 100 items",
				"Processing 200 items",
				"Processing 500 items",
			},
			expectedTokens: []token.TokenType{
				token.TokenWord,
				token.TokenWhitespace,
				token.TokenHTTPStatus,
				token.TokenWhitespace,
				token.TokenWord,
			},
			description:          "Logs with different numeric values should cluster together",
			expectedPatternCount: 1,
		},
		{
			name: "Complex log with multiple token types",
			logs: []string{
				"2024-01-15 10:30:00 INFO User admin@example.com from 192.168.1.1 GET /api/users 200",
				"2024-01-15 10:31:00 INFO User user@test.com from 10.0.0.1 POST /api/users 201",
			},
			expectedTokens: []token.TokenType{
				token.TokenLocalDateTime,
				token.TokenWhitespace,
				token.TokenSeverityLevel,
				token.TokenWhitespace,
				token.TokenWord,
				token.TokenEmail,
				token.TokenWhitespace,
				token.TokenWord,
				token.TokenWhitespace,
				token.TokenIPv4,
				token.TokenWhitespace,
				token.TokenHTTPMethod,
				token.TokenWhitespace,
				token.TokenAbsolutePath,
				token.TokenWhitespace,
				token.TokenHTTPStatus,
			},
			description:          "Complex logs with multiple token types should cluster correctly",
			expectedPatternCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset cluster manager for each test
			cm = clustering.NewClusterManager()

			var patterns []*clustering.Pattern
			var tokenLists []*token.TokenList

			// Tokenize all logs
			for _, log := range tt.logs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err, "Tokenization should succeed for log: %q", log)
				require.NotNil(t, tokenList, "TokenList should not be nil")
				require.Greater(t, tokenList.Length(), 0, "TokenList should have at least one token")

				tokenLists = append(tokenLists, tokenList)

				// Verify expected token types are present
				foundTypes := make(map[token.TokenType]bool)
				for i := 0; i < tokenList.Length(); i++ {
					foundTypes[tokenList.Tokens[i].Type] = true
				}

				// Check that we have all expected token types
				for _, expectedType := range tt.expectedTokens {
					assert.True(t, foundTypes[expectedType],
						"Log %q should contain token type %v. Found: %v. Expected: %v",
						log, expectedType, foundTypes, tt.expectedTokens)
				}

				// Spot-check raw values for offset correctness where applicable
				if strings.Contains(log, "GET ") {
					assertContainsTrimmed(t, tokenValuesOfType(tokenList, token.TokenHTTPMethod), "GET",
						"HTTP method token should preserve raw value from log")
				}
				if strings.Contains(log, "POST ") {
					assertContainsTrimmed(t, tokenValuesOfType(tokenList, token.TokenHTTPMethod), "POST",
						"HTTP method token should preserve raw value from log")
				}
				if strings.Contains(log, "PUT ") {
					assertContainsTrimmed(t, tokenValuesOfType(tokenList, token.TokenHTTPMethod), "PUT",
						"HTTP method token should preserve raw value from log")
				}

				// Add to clustering
				pattern, changeType, _, _ := cm.Add(tokenList)
				require.NotNil(t, pattern, "Pattern should not be nil after adding to clustering")
				patterns = append(patterns, pattern)

				// Verify pattern has correct structure
				assert.NotNil(t, pattern.Template, "Pattern template should not be nil")
				assert.Greater(t, pattern.LogCount, 0, "Pattern should have log count > 0")
				assert.Contains(t, []clustering.PatternChangeType{
					clustering.PatternNew,
					clustering.PatternUpdated,
					clustering.PatternNoChange,
				}, changeType, "Pattern change type should be valid")
			}

			// Verify clustering behavior
			assert.Greater(t, cm.PatternCount(), 0, "ClusterManager should have at least one pattern")
			assert.Greater(t, len(patterns), 0, "Should have created at least one pattern")

			// Verify that similar logs cluster together
			// (logs with same structure but different values should be in same or related patterns)
			if len(tt.logs) > 1 && tt.expectedPatternCount > 0 {
				assert.Equal(t, tt.expectedPatternCount, cm.PatternCount(),
					"Expected %d patterns after clustering, got %d", tt.expectedPatternCount, cm.PatternCount())

				// All patterns should have similar structure (same token types)
				firstPattern := patterns[0]
				for i := 1; i < len(patterns); i++ {
					otherPattern := patterns[i]
					assert.Equal(t, firstPattern.Template.Length(), otherPattern.Template.Length(),
						"Patterns from similar logs should have same token count")
				}
			}

			t.Logf("✅ %s: Processed %d logs, created %d patterns, total patterns in manager: %d",
				tt.description, len(tt.logs), len(patterns), cm.PatternCount())
		})
	}
}

func tokenValuesOfType(tl *token.TokenList, tokType token.TokenType) []string {
	values := []string{}
	for _, tok := range tl.Tokens {
		if tok.Type == tokType {
			values = append(values, tok.Value)
		}
	}
	return values
}

func assertContainsTrimmed(t *testing.T, values []string, expected string, msgAndArgs ...interface{}) {
	t.Helper()
	for _, v := range values {
		if strings.TrimSpace(v) == expected {
			return
		}
	}
	assert.Fail(t, "Expected value not found after trimming", msgAndArgs...)
}

// TestRustTokenizer_Clustering_AllTokenTypes validates that all token types
// can be successfully tokenized and added to clustering without errors.
func TestRustTokenizer_Clustering_AllTokenTypes(t *testing.T) {
	tokenizer := NewRustTokenizer()
	cm := clustering.NewClusterManager()

	// Test logs that should produce each token type
	// Note: Some token types are flattened into components by the Rust tokenizer
	testCases := []struct {
		name        string
		log         string
		expectTypes []token.TokenType
		description string
	}{
		{"Word", "Hello World", []token.TokenType{token.TokenWord}, "Basic word tokens"},
		{"Whitespace", "Hello  World", []token.TokenType{token.TokenWhitespace}, "Whitespace tokens"},
		{"SpecialChar", "Hello, World!", []token.TokenType{token.TokenSpecialChar}, "Special character tokens"},
		{"Numeric", "Count: 123", []token.TokenType{token.TokenNumeric}, "Numeric value tokens"},
		{"HTTPMethod", "GET /api", []token.TokenType{token.TokenHTTPMethod}, "HTTP method tokens"},
		{"HTTPStatus", "Status: 200", []token.TokenType{token.TokenHTTPStatus}, "HTTP status tokens"},
		{"Severity", "ERROR Failed", []token.TokenType{token.TokenSeverityLevel}, "Severity level tokens"},
		{"IPv4", "IP: 192.168.1.1", []token.TokenType{token.TokenIPv4}, "IPv4 address tokens"},
		{"IPv6_flattened", "IP: 2001:0db8::1", []token.TokenType{token.TokenNumeric, token.TokenSpecialChar, token.TokenWord}, "IPv6 flattened to Numeric+SpecialChar+Word components"},
		{"Email", "Contact: user@example.com", []token.TokenType{token.TokenEmail}, "Email address tokens"},
		{"URI", "URL: https://example.com", []token.TokenType{token.TokenURI}, "URI tokens"},
		{"AbsolutePath", "Path: /api/users", []token.TokenType{token.TokenAbsolutePath}, "Absolute path tokens"},
		{"Authority_flattened", "Host: example.com:8080", []token.TokenType{token.TokenWord, token.TokenSpecialChar, token.TokenNumeric}, "Authority flattened to Word+SpecialChar+Numeric components"},
		{"LocalDateTime_consolidated", "Date: 2024-01-15", []token.TokenType{token.TokenLocalDateTime}, "LocalDate consolidated to LocalDateTime"},
		{"LocalDateTime", "Time: 2024-01-15 10:30:00", []token.TokenType{token.TokenLocalDateTime}, "LocalDateTime tokens"},
		{"RegularName_flattened", "Name: my-service-123", []token.TokenType{token.TokenWord, token.TokenSpecialChar}, "RegularName flattened to Word+SpecialChar components"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(tc.log)
			require.NoError(t, err, "Tokenization should succeed for %s", tc.name)
			require.NotNil(t, tokenList, "TokenList should not be nil")
			require.Greater(t, tokenList.Length(), 0, "TokenList should have tokens")

			// Verify expected token types are present
			foundTypes := make(map[token.TokenType]bool)
			for i := 0; i < tokenList.Length(); i++ {
				foundTypes[tokenList.Tokens[i].Type] = true
			}

			for _, expectedType := range tc.expectTypes {
				assert.True(t, foundTypes[expectedType],
					"Token type %v should be present in tokenized log %q. Found types: %v. %s",
					expectedType, tc.log, foundTypes, tc.description)
			}

			// Add to clustering - should not panic or error
			pattern, changeType, _, _ := cm.Add(tokenList)
			require.NotNil(t, pattern, "Pattern should not be nil")
			assert.Contains(t, []clustering.PatternChangeType{
				clustering.PatternNew,
				clustering.PatternUpdated,
				clustering.PatternNoChange,
			}, changeType, "Pattern change type should be valid")

			t.Logf("✅ %s token type works: tokenized %q → %d tokens → pattern with %d tokens",
				tc.name, tc.log, tokenList.Length(), pattern.Template.Length())
		})
	}

	assert.Greater(t, cm.PatternCount(), 0, "Should have created patterns for all token types")
	t.Logf("✅ All token types successfully integrated with clustering")
}
