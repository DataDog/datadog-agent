// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automaton

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestTokenizer_SimpleTokenization tests basic tokenization and type classification
func TestTokenizer_SimpleTokenization(t *testing.T) {
	input := "GET /api 200"
	tokenizer := newTokenizerInternal(input)
	defer tokenizer.Release()
	tokenList := tokenizer.tokenize()

	assert.NotEqual(t, 0, tokenList.Length(), "Expected tokens, got empty list")

	// Should have: GET, whitespace, /api, whitespace, 200
	assert.Equal(t, 5, tokenList.Length(), "Expected 5 tokens")

	// Verify token types
	expectedTypes := []token.TokenType{
		token.TokenHTTPMethod,   // GET
		token.TokenWhitespace,   // space
		token.TokenAbsolutePath, // /api
		token.TokenWhitespace,   // space
		token.TokenHTTPStatus,   // 200
	}

	for i, expected := range expectedTypes {
		if assert.Less(t, i, tokenList.Length(), "Token %d should exist", i) {
			assert.Equal(t, expected, tokenList.Tokens[i].Type,
				"Token %d (value: '%s') should be type %v", i, tokenList.Tokens[i].Value, expected)
		}
	}
}

// TestTokenizer_StateTransitions tests the state transitions of the tokenizer
func TestTokenizer_StateTransitions(t *testing.T) {
	tests := []struct {
		input          string
		expectedStates []TokenizerState
		description    string
	}{
		{"GET", []TokenizerState{StateStart, StateWord}, "Simple word"},
		{"123", []TokenizerState{StateStart, StateNumeric}, "Simple numeric"},
		{" ", []TokenizerState{StateStart, StateWhitespace}, "Single whitespace"},
		{"/path", []TokenizerState{StateStart, StateWord}, "Path starts as word character"},
		{"192.168.1.1", []TokenizerState{StateStart, StateNumeric}, "IPv4 stays in numeric state initially"},
	}

	for _, test := range tests {
		tokenizer := newTokenizerInternal(test.input)

		// Capture state transitions
		var states []TokenizerState
		states = append(states, tokenizer.state)

		for tokenizer.pos < tokenizer.length {
			if !tokenizer.processNextToken() {
				break
			}
			states = append(states, tokenizer.state)
		}

		// For simple cases, check exact sequence
		if test.input != "192.168.1.1" {
			assert.GreaterOrEqual(t, len(states), len(test.expectedStates),
				"Input '%s' (%s): expected at least %d states", test.input, test.description, len(test.expectedStates))

			// Check that expected states appear in sequence
			for i, expected := range test.expectedStates {
				if assert.Less(t, i, len(states), "State %d should exist for input '%s'", i, test.input) {
					assert.Equal(t, expected, states[i],
						"Input '%s' (%s): expected state %d to be %v", test.input, test.description, i, expected)
				}
			}
		} else {
			// For IPv4 with simplified FSA, check basic state transitions
			hasStart := false
			hasNumeric := false

			for _, state := range states {
				switch state {
				case StateStart:
					hasStart = true
				case StateNumeric:
					hasNumeric = true
				}
			}

			assert.True(t, hasStart, "IPv4 test: expected to see StateStart")
			assert.True(t, hasNumeric, "IPv4 test: expected to see StateNumeric")
		}

		tokenizer.Release()
	}
}

// TestTokenTypePreservation tests that TokenNumeric stays TokenNumeric when no pattern matches
// This is critical: classification should upgrade OR preserve, never downgrade
func TestTokenTypePreservation(t *testing.T) {
	// Test that generic number stays TokenNumeric (not downgraded to TokenWord)
	tokenList := TokenizeString("User 12345 logged in")

	// Find the numeric token
	var numericToken *token.Token
	for i := range tokenList.Tokens {
		if tokenList.Tokens[i].Value == "12345" {
			numericToken = &tokenList.Tokens[i]
			break
		}
	}

	assert.NotNil(t, numericToken, "Expected to find numeric token '12345'")

	// Should stay TokenNumeric, not become TokenWord
	if numericToken != nil {
		assert.Equal(t, token.TokenNumeric, numericToken.Type,
			"Token '12345' should stay TokenNumeric")
	}

	// Test that numeric upgrades when pattern matches
	tokenList = TokenizeString("User 192.168.1.1 logged in")

	// Find the IP token
	var ipToken *token.Token
	for i := range tokenList.Tokens {
		if tokenList.Tokens[i].Value == "192.168.1.1" {
			ipToken = &tokenList.Tokens[i]
			break
		}
	}

	assert.NotNil(t, ipToken, "Expected to find IP token '192.168.1.1'")

	// Should be upgraded to TokenIPv4
	if ipToken != nil {
		assert.Equal(t, token.TokenIPv4, ipToken.Type,
			"Token '192.168.1.1' should be TokenIPv4")
	}
}

// TestWildcardStatus tests that tokens are correctly marked as NotWildcard or PotentialWildcard
func TestWildcardStatus(t *testing.T) {
	tests := []struct {
		input            string
		tokenValue       string
		expectedWildcard token.WildcardStatus
		description      string
	}{
		{" ", " ", token.NotWildcard, "Whitespace should be NotWildcard"},
		{":", ":", token.NotWildcard, "Punctuation should be NotWildcard"},
		{"hello", "hello", token.PotentialWildcard, "Generic word should be PotentialWildcard"},
		{"12345", "12345", token.PotentialWildcard, "Generic number should be PotentialWildcard"},
		{"INFO User logged in", "INFO", token.PotentialWildcard, "Severity level should be PotentialWildcard"},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			tokenList := TokenizeString(test.input)

			var targetToken *token.Token
			for i := range tokenList.Tokens {
				if tokenList.Tokens[i].Value == test.tokenValue {
					targetToken = &tokenList.Tokens[i]
					break
				}
			}

			assert.NotNil(t, targetToken, "Expected to find token '%s'", test.tokenValue)

			if targetToken != nil {
				assert.Equal(t, test.expectedWildcard, targetToken.Wildcard, test.description)
			}
		})
	}
}

// Test the complete data flow
func TestArchitectureCompliance(t *testing.T) {
	// Test the exact call graph
	// automaton.TokenizeString → NewTokenizer → Tokenizer.Tokenize → processNextToken → classifyToken → globalTrie.Match

	input := "GET /api/users 200"

	// Step 1: automaton.TokenizeString (main entry point)
	tokenList := TokenizeString(input)

	// Verify TokenList creation
	assert.NotNil(t, tokenList, "TokenizeString returned nil")

	// Step 2: Verify token classification used globalTrie.Match
	var httpMethod, httpStatus, path *token.Token

	for i := range tokenList.Tokens {
		switch tokenList.Tokens[i].Type {
		case token.TokenHTTPMethod:
			httpMethod = &tokenList.Tokens[i]
		case token.TokenHTTPStatus:
			httpStatus = &tokenList.Tokens[i]
		case token.TokenAbsolutePath:
			path = &tokenList.Tokens[i]
		}
	}

	if assert.NotNil(t, httpMethod, "HTTP method token not found - trie classification failed") {
		assert.Equal(t, "GET", httpMethod.Value, "Expected HTTP method 'GET'")
	}

	if assert.NotNil(t, httpStatus, "HTTP status token not found - trie classification failed") {
		assert.Equal(t, "200", httpStatus.Value, "Expected HTTP status '200'")
	}

	if assert.NotNil(t, path, "Path token not found - state machine failed") {
		assert.Equal(t, "/api/users", path.Value, "Expected path '/api/users'")
	}

	// Step 3: Verify signature generation works
	signature := token.NewSignature(tokenList)
	assert.False(t, signature.IsEmpty(), "Signature generation failed")

	expectedPosition := "HTTPMethod|Whitespace|AbsolutePath|Whitespace|HTTPStatus"
	assert.Equal(t, expectedPosition, signature.Position, "Signature position mismatch")
}

// TestComplexLogScenarios tests complex log scenarios
func TestComplexLogScenarios(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []token.TokenType
	}{
		{
			name:  "HTTP Request",
			input: "GET /api/users 200",
			expected: []token.TokenType{
				token.TokenHTTPMethod, token.TokenWhitespace,
				token.TokenAbsolutePath, token.TokenWhitespace,
				token.TokenHTTPStatus,
			},
		},
		{
			name:  "Error Message",
			input: "ERROR Database connection failed",
			expected: []token.TokenType{
				token.TokenSeverityLevel, token.TokenWhitespace,
				token.TokenWord, token.TokenWhitespace,
				token.TokenWord, token.TokenWhitespace,
				token.TokenWord,
			},
		},
		{
			name:  "User Login",
			input: "INFO User 12345 logged in",
			expected: []token.TokenType{
				token.TokenSeverityLevel, token.TokenWhitespace,
				token.TokenWord, token.TokenWhitespace,
				token.TokenNumeric, token.TokenWhitespace,
				token.TokenWord, token.TokenWhitespace,
				token.TokenWord,
			},
		},
		{
			name:  "Complex with Email and IP",
			input: "user@domain.com from 192.168.1.1",
			expected: []token.TokenType{
				token.TokenEmail, token.TokenWhitespace,
				token.TokenWord, token.TokenWhitespace,
				token.TokenIPv4,
			},
		},
		{
			name:  "URL with Scheme",
			input: "Visit https://example.com/docs",
			expected: []token.TokenType{
				token.TokenWord, token.TokenWhitespace,
				token.TokenURI,
			},
		},
		{
			name:  "Date in Context",
			input: "Event on 2024-01-15",
			expected: []token.TokenType{
				token.TokenWord, token.TokenWhitespace,
				token.TokenWord, token.TokenWhitespace,
				token.TokenDate,
			},
		},
		{
			name:  "False Positive - Single @ is not Email",
			input: "Price @ $10 each",
			expected: []token.TokenType{
				token.TokenWord,       // Price
				token.TokenWhitespace, // space
				token.TokenWord,       // @
				token.TokenWhitespace, // space
				token.TokenWord,       // $
				token.TokenNumeric,    // 10
				token.TokenWhitespace, // space
				token.TokenWord,       // each
			},
		},
		{
			name:  "False Positive - Division operator is not Path",
			input: "Calculate 10 / 2 = 5",
			expected: []token.TokenType{
				token.TokenWord,       // Calculate
				token.TokenWhitespace, // space
				token.TokenNumeric,    // 10
				token.TokenWhitespace, // space
				token.TokenWord,       // /
				token.TokenWhitespace, // space
				token.TokenNumeric,    // 2
				token.TokenWhitespace, // space
				token.TokenWord,       // =
				token.TokenWhitespace, // space
				token.TokenNumeric,    // 5
			},
		},
		{
			name:  "False Positive - Phone number is not Date",
			input: "Phone: 123-456-7890",
			expected: []token.TokenType{
				token.TokenWord,       // Phone
				token.TokenWord,       // :
				token.TokenWhitespace, // space
				token.TokenNumeric,    // 123-456-7890 stays numeric, not date
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokenList := TokenizeString(test.input)

			assert.Equal(t, len(test.expected), tokenList.Length(),
				"Expected %d tokens, got: %v", len(test.expected), tokenTypesToString(tokenList.Tokens))

			for i, expected := range test.expected {
				if assert.Less(t, i, tokenList.Length(), "Token %d should exist", i) {
					assert.Equal(t, expected, tokenList.Tokens[i].Type,
						"Token %d (value: '%s') should be type %v", i, tokenList.Tokens[i].Value, expected)
				}
			}
		})
	}
}

// TestWhitespaceNormalization tests that all whitespace types are normalized to single space
func TestWhitespaceNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Single space",
			input:    "Error: message",
			expected: " ",
		},
		{
			name:     "Tab character",
			input:    "Error:\tmessage",
			expected: " ",
		},
		{
			name:     "Multiple spaces",
			input:    "Error:  message",
			expected: " ",
		},
		{
			name:     "Multiple tabs",
			input:    "Error:\t\tmessage",
			expected: " ",
		},
		{
			name:     "Mixed tabs and spaces",
			input:    "Error: \t message",
			expected: " ",
		},
		{
			name:     "Newline",
			input:    "Error:\nmessage",
			expected: " ",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokenList := TokenizeString(test.input)

			// Find whitespace token
			var whitespaceToken *token.Token
			for i := range tokenList.Tokens {
				if tokenList.Tokens[i].Type == token.TokenWhitespace {
					whitespaceToken = &tokenList.Tokens[i]
					break
				}
			}

			assert.NotNil(t, whitespaceToken, "Expected to find whitespace token")

			if whitespaceToken != nil {
				assert.Equal(t, test.expected, whitespaceToken.Value,
					"Whitespace should be normalized to single space")
				assert.Equal(t, token.NotWildcard, whitespaceToken.Wildcard,
					"Whitespace should be NotWildcard")
			}
		})
	}
}

// TestWhitespaceNormalization_Signature tests if whitespace normalization would allows logs with different whitespace to merge into the same pattern
func TestWhitespaceNormalization_Signature(t *testing.T) {
	// These logs differ only in whitespace - they should tokenize identically
	log1 := "Error: connection failed"  // single space
	log2 := "Error:\tconnection failed" // tab
	log3 := "Error:  connection failed" // double space

	tl1 := TokenizeString(log1)
	tl2 := TokenizeString(log2)
	tl3 := TokenizeString(log3)

	// All should have same token count
	assert.Equal(t, tl1.Length(), tl2.Length(), "Token counts should match")
	assert.Equal(t, tl1.Length(), tl3.Length(), "Token counts should match")

	// All whitespace tokens should be normalized to single space
	for i := 0; i < tl1.Length(); i++ {
		if tl1.Tokens[i].Type == token.TokenWhitespace {
			assert.Equal(t, " ", tl1.Tokens[i].Value, "Whitespace in log1 should be normalized")
			assert.Equal(t, " ", tl2.Tokens[i].Value, "Whitespace in log2 should be normalized")
			assert.Equal(t, " ", tl3.Tokens[i].Value, "Whitespace in log3 should be normalized")
		}
	}

	// Signatures should be identical
	sig1 := token.NewSignature(tl1)
	sig2 := token.NewSignature(tl2)
	sig3 := token.NewSignature(tl3)

	assert.True(t, sig1.Equals(sig2), "Signatures should be equal after normalization")
	assert.True(t, sig1.Equals(sig3), "Signatures should be equal after normalization")
}

// ===============================
// Helper functions
// ===============================
func tokenTypesToString(tokens []token.Token) []string {
	result := make([]string, len(tokens))
	for i, tok := range tokens {
		result[i] = tok.String()
	}
	return result
}
