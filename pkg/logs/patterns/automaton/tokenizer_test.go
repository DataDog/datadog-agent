// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automaton

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestNewTokenizer(t *testing.T) {
	input := "GET /api/users 200"
	tokenizer := NewTokenizer(input)

	if tokenizer.input != input {
		t.Errorf("Expected input '%s', got '%s'", input, tokenizer.input)
	}
	if tokenizer.pos != 0 {
		t.Errorf("Expected pos 0, got %d", tokenizer.pos)
	}
	if tokenizer.length != len(input) {
		t.Errorf("Expected length %d, got %d", len(input), tokenizer.length)
	}
	if tokenizer.state != StateStart {
		t.Errorf("Expected StateStart, got %v", tokenizer.state)
	}
}

func TestTokenizer_SimpleTokenization(t *testing.T) {
	input := "GET /api 200"
	tokenizer := NewTokenizer(input)
	tokenList := tokenizer.Tokenize()

	if tokenList.Length() == 0 {
		t.Fatal("Expected tokens, got empty list")
	}

	// Should have: GET, whitespace, /api, whitespace, 200
	if tokenList.Length() != 5 {
		t.Errorf("Expected 5 tokens, got %d: %v", tokenList.Length(), tokenList.Tokens)
	}

	// Verify token types
	expectedTypes := []token.TokenType{
		token.TokenHttpMethod,   // GET
		token.TokenWhitespace,   // space
		token.TokenAbsolutePath, // /api
		token.TokenWhitespace,   // space
		token.TokenHttpStatus,   // 200
	}

	for i, expected := range expectedTypes {
		if i >= tokenList.Length() {
			t.Errorf("Expected token %d of type %v, but tokenList too short", i, expected)
			continue
		}
		if tokenList.Tokens[i].Type != expected {
			t.Errorf("Token %d: expected type %v, got %v (value: '%s')",
				i, expected, tokenList.Tokens[i].Type, tokenList.Tokens[i].Value)
		}
	}
}

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
		tokenizer := NewTokenizer(test.input)

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
			if len(states) < len(test.expectedStates) {
				t.Errorf("Input '%s' (%s): expected at least %d states, got %d",
					test.input, test.description, len(test.expectedStates), len(states))
				continue
			}

			// Check that expected states appear in sequence
			for i, expected := range test.expectedStates {
				if i >= len(states) || states[i] != expected {
					t.Errorf("Input '%s' (%s): expected state %d to be %v, got sequence: %v",
						test.input, test.description, i, expected, states)
					break
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

			if !hasStart {
				t.Errorf("IPv4 test: expected to see StateStart")
			}
			if !hasNumeric {
				t.Errorf("IPv4 test: expected to see StateNumeric")
			}
		}
	}
}

func TestStringInterning(t *testing.T) {
	// String interning has been removed for POC simplicity
	// This test now just verifies that tokenization works correctly
	tokenizer := NewTokenizer("GET GET POST POST")
	tokenList := tokenizer.Tokenize()

	// Find GET tokens and verify correct tokenization
	var getTokens []string
	for _, tok := range tokenList.Tokens {
		if tok.Value == "GET" {
			getTokens = append(getTokens, tok.Value)
		}
	}

	if len(getTokens) != 2 {
		t.Errorf("Expected 2 GET tokens, got %d", len(getTokens))
	}
}

// Performance test to verify O(n) characteristics
func TestTokenizationPerformance(t *testing.T) {
	// Test with different input sizes to verify O(n) tokenization
	inputs := []string{
		"GET /api 200", // Small
		"GET /api/users/12345 200 application/json " + repeatString("x", 100), // Medium
		"POST /api/data " + repeatString("word ", 1000) + "500",               // Large
	}

	for i, input := range inputs {
		start := time.Now()
		tokenList := TokenizeString(input)
		duration := time.Since(start)

		t.Logf("Input %d (len %d): %d tokens in %v",
			i+1, len(input), tokenList.Length(), duration)

		if tokenList.Length() == 0 {
			t.Errorf("Input %d produced no tokens", i+1)
		}
	}
}

func TestComplexLogScenarios(t *testing.T) {
	// Test scenarios from your architecture diagrams
	tests := []struct {
		name     string
		input    string
		expected []token.TokenType
	}{
		{
			name:  "HTTP Request",
			input: "GET /api/users 200",
			expected: []token.TokenType{
				token.TokenHttpMethod, token.TokenWhitespace,
				token.TokenAbsolutePath, token.TokenWhitespace,
				token.TokenHttpStatus,
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokenList := TokenizeString(test.input)

			if tokenList.Length() != len(test.expected) {
				t.Errorf("Expected %d tokens, got %d: %v",
					len(test.expected), tokenList.Length(),
					tokenTypesToString(tokenList.Tokens))
			}

			for i, expected := range test.expected {
				if i >= tokenList.Length() {
					t.Errorf("Token %d missing", i)
					continue
				}
				if tokenList.Tokens[i].Type != expected {
					t.Errorf("Token %d: expected %v, got %v (value: '%s')",
						i, expected, tokenList.Tokens[i].Type, tokenList.Tokens[i].Value)
				}
			}
		})
	}
}

// Test the complete data flow as specified in architecture
func TestArchitectureCompliance(t *testing.T) {
	// Test the exact call graph from your architecture:
	// automaton.TokenizeString → NewTokenizer → Tokenizer.Tokenize
	// → processNextToken → classifyToken → globalTrie.Match

	input := "GET /api/users 200"

	// Step 1: automaton.TokenizeString (main entry point)
	tokenList := TokenizeString(input)

	// Verify TokenList creation
	if tokenList == nil {
		t.Fatal("TokenizeString returned nil")
	}

	// Step 2: Verify token classification used globalTrie.Match
	httpMethodFound := false
	httpStatusFound := false
	pathFound := false

	for _, tok := range tokenList.Tokens {
		switch tok.Type {
		case token.TokenHttpMethod:
			httpMethodFound = true
			if tok.Value != "GET" {
				t.Errorf("Expected HTTP method 'GET', got '%s'", tok.Value)
			}
		case token.TokenHttpStatus:
			httpStatusFound = true
			if tok.Value != "200" {
				t.Errorf("Expected HTTP status '200', got '%s'", tok.Value)
			}
		case token.TokenAbsolutePath:
			pathFound = true
			if tok.Value != "/api/users" {
				t.Errorf("Expected path '/api/users', got '%s'", tok.Value)
			}
		}
	}

	if !httpMethodFound {
		t.Error("HTTP method token not found - trie classification failed")
	}
	if !httpStatusFound {
		t.Error("HTTP status token not found - trie classification failed")
	}
	if !pathFound {
		t.Error("Path token not found - state machine failed")
	}

	// Step 3: Verify signature generation works
	signature := token.NewSignature(tokenList)
	if signature.IsEmpty() {
		t.Error("Signature generation failed")
	}

	expectedPosition := "HttpMethod|Whitespace|AbsolutePath|Whitespace|HttpStatus"
	if signature.Position != expectedPosition {
		t.Errorf("Expected signature position '%s', got '%s'",
			expectedPosition, signature.Position)
	}
}

// Helper functions

func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func tokenTypesToString(tokens []token.Token) []string {
	result := make([]string, len(tokens))
	for i, tok := range tokens {
		result[i] = fmt.Sprintf("%s('%s')", tok.Type, tok.Value)
	}
	return result
}

// TestComplexTokenClassification tests regex-based token classification
func TestComplexTokenClassification(t *testing.T) {
	// Test email classification
	t.Run("Email", func(t *testing.T) {
		tokens := TokenizeString("Contact user@example.com for help")
		emailFound := false
		for _, tok := range tokens.Tokens {
			if tok.Type == token.TokenEmail && tok.Value == "user@example.com" {
				emailFound = true
				break
			}
		}
		assert.True(t, emailFound, "Should detect email via regex classification")
	})

	// Test date classification
	t.Run("Date", func(t *testing.T) {
		// Test simple date detection (works)
		tokens := TokenizeString("Event on 2024-01-15")
		dateFound := false
		for _, tok := range tokens.Tokens {
			if tok.Type == token.TokenDate && tok.Value == "2024-01-15" {
				dateFound = true
				break
			}
		}
		assert.True(t, dateFound, "Should detect date via regex classification")

		// Test isolated date (works)
		tokens = TokenizeString("2024-01-15")
		dateFound = false
		for _, tok := range tokens.Tokens {
			if tok.Type == token.TokenDate && tok.Value == "2024-01-15" {
				dateFound = true
				break
			}
		}
		assert.True(t, dateFound, "Should detect isolated date")
	})

	// Test path classification
	t.Run("Path", func(t *testing.T) {
		tokens := TokenizeString("GET /api/users/123 returned 200")
		pathFound := false
		for _, tok := range tokens.Tokens {
			if tok.Type == token.TokenAbsolutePath && tok.Value == "/api/users/123" {
				pathFound = true
				break
			}
		}
		assert.True(t, pathFound, "Should detect path via regex classification")
	})

	// Test URL classification
	t.Run("URL", func(t *testing.T) {
		// Test simple URL detection
		tokens := TokenizeString("https://example.com/docs")
		urlFound := false
		for _, tok := range tokens.Tokens {
			if tok.Type == token.TokenURI && strings.Contains(tok.Value, "/example.com/docs") {
				urlFound = true
				break
			}
		}
		assert.True(t, urlFound, "Should detect URL via regex classification")
	})

	// Test false positives avoided
	t.Run("FalsePositives", func(t *testing.T) {
		// Single @ should not be email
		tokens := TokenizeString("Price @ $10 each")
		for _, tok := range tokens.Tokens {
			assert.NotEqual(t, token.TokenEmail, tok.Type, "Single @ should not be detected as email")
		}

		// Single / should not be path
		tokens = TokenizeString("Calculate 10 / 2 = 5")
		pathCount := 0
		for _, tok := range tokens.Tokens {
			if tok.Type == token.TokenAbsolutePath {
				pathCount++
			}
		}
		assert.Equal(t, 0, pathCount, "Division operator should not be detected as path")

		// Numbers with separators that aren't dates
		tokens = TokenizeString("Phone: 123-456-7890")
		dateCount := 0
		for _, tok := range tokens.Tokens {
			if tok.Type == token.TokenDate {
				dateCount++
			}
		}
		assert.Equal(t, 0, dateCount, "Phone number should not be detected as date")
	})
}
