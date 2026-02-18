//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtokenizer provides tests for the token conversion layer.
//
// This file contains UNIT TESTS for the Go conversion functions that translate
// Rust token types to Agent token types. These tests do NOT call Rust FFI.
//
// For END-TO-END integration tests (FFI + tokenization), see tokenizer_test.go.

package rtokenizer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestRustTokenToAgentToken(t *testing.T) {
	tests := []struct {
		name          string
		rustToken     RustToken
		expectedType  token.TokenType
		expectedValue string
		expectedWC    token.WildcardStatus
	}{
		{
			name: "Word token without digits",
			rustToken: RustToken{
				Kind:          RustTokenWord,
				Value:         "hello",
				NeverWildcard: false,
				HasDigits:     false,
			},
			expectedType:  token.TokenWord,
			expectedValue: "hello",
			expectedWC:    token.PotentialWildcard,
		},
		{
			name: "Word token with never_wildcard",
			rustToken: RustToken{
				Kind:          RustTokenWord,
				Value:         "ERROR",
				NeverWildcard: true,
				HasDigits:     false,
			},
			expectedType:  token.TokenWord,
			expectedValue: "ERROR",
			expectedWC:    token.NotWildcard,
		},
		{
			name: "Whitespace token",
			rustToken: RustToken{
				Kind:  RustTokenWhitespace,
				Value: " ",
			},
			expectedType:  token.TokenWhitespace,
			expectedValue: " ",
			expectedWC:    token.NotWildcard,
		},
		{
			name: "Numeric token",
			rustToken: RustToken{
				Kind:      RustTokenNumeric,
				Value:     "123",
				HasDigits: true,
			},
			expectedType:  token.TokenNumeric,
			expectedValue: "123",
			expectedWC:    token.PotentialWildcard,
		},
		{
			name: "HTTP Status token",
			rustToken: RustToken{
				Kind:  RustTokenHTTPStatus,
				Value: "200",
			},
			expectedType:  token.TokenHTTPStatus,
			expectedValue: "200",
			expectedWC:    token.PotentialWildcard,
		},
		{
			name: "IPv4 token",
			rustToken: RustToken{
				Kind:  RustTokenIPv4,
				Value: "192.168.1.1",
			},
			expectedType:  token.TokenIPv4,
			expectedValue: "192.168.1.1",
			expectedWC:    token.PotentialWildcard,
		},
		{
			name: "Local date token",
			rustToken: RustToken{
				Kind:  RustTokenLocalDate,
				Value: "yyyy-MM-dd",
			},
			expectedType:  token.TokenLocalDate,
			expectedValue: "yyyy-MM-dd",
			expectedWC:    token.PotentialWildcard,
		},
		{
			name: "Unknown token",
			rustToken: RustToken{
				Kind:  RustTokenUnknown,
				Value: "???",
			},
			expectedType:  token.TokenUnknown,
			expectedValue: "???",
			expectedWC:    token.PotentialWildcard,
		},
		{
			name: "Collapsed token",
			rustToken: RustToken{
				Kind:  RustTokenCollapsed,
				Value: "*",
			},
			expectedType:  token.TokenCollapsedToken,
			expectedValue: "*",
			expectedWC:    token.PotentialWildcard,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentToken := tt.rustToken.ToAgentToken()

			if agentToken.Type != tt.expectedType {
				t.Errorf("Expected type %v, got %v", tt.expectedType, agentToken.Type)
			}
			if agentToken.Value != tt.expectedValue {
				t.Errorf("Expected value %q, got %q", tt.expectedValue, agentToken.Value)
			}
			if agentToken.Wildcard != tt.expectedWC {
				t.Errorf("Expected wildcard status %v, got %v", tt.expectedWC, agentToken.Wildcard)
			}
			if agentToken.NeverWildcard != tt.rustToken.NeverWildcard {
				t.Errorf("Expected NeverWildcard %v, got %v", tt.rustToken.NeverWildcard, agentToken.NeverWildcard)
			}
			if agentToken.HasDigits != tt.rustToken.HasDigits {
				t.Errorf("Expected HasDigits %v, got %v", tt.rustToken.HasDigits, agentToken.HasDigits)
			}
		})
	}
}

func TestToAgentTokenWithRaw_UsesOffsets(t *testing.T) {
	setExtractFromOriginalLog(true)

	rt := RustToken{
		Kind:        RustTokenHTTPMethod,
		Value:       "Get",
		StartOffset: 0,
		EndOffset:   3,
	}
	raw := "GET /api"

	tok := rt.ToAgentTokenWithRaw(raw)
	if tok.Value != "GET" {
		t.Errorf("Expected raw value %q, got %q", "GET", tok.Value)
	}
}

func TestToAgentTokenWithRaw_Disabled(t *testing.T) {
	setExtractFromOriginalLog(false)
	defer setExtractFromOriginalLog(true)

	rt := RustToken{
		Kind:        RustTokenHTTPMethod,
		Value:       "Get",
		StartOffset: 0,
		EndOffset:   3,
	}
	raw := "GET /api"

	tok := rt.ToAgentTokenWithRaw(raw)
	if tok.Value != "Get" {
		t.Errorf("Expected normalized value %q when raw offsets disabled, got %q", "Get", tok.Value)
	}
}

func TestToAgentTokenWithRaw_InvalidOffsets(t *testing.T) {
	setExtractFromOriginalLog(true)

	rt := RustToken{
		Kind:        RustTokenHTTPMethod,
		Value:       "Get",
		StartOffset: 10,
		EndOffset:   20,
	}
	raw := "GET /api"

	tok := rt.ToAgentTokenWithRaw(raw)
	if tok.Value != "Get" {
		t.Errorf("Expected normalized value when offsets invalid, got %q", tok.Value)
	}
}

func TestRustTokensToTokenList(t *testing.T) {
	tests := []struct {
		name           string
		rustTokens     []RustToken
		expectedLen    int
		expectedTypes  []token.TokenType
		expectedValues []string
	}{
		{
			name: "Simple log with date and severity",
			rustTokens: []RustToken{
				{Kind: RustTokenLocalDate, Value: "yyyy-MM-dd"},
				{Kind: RustTokenWhitespace, Value: " "},
				{Kind: RustTokenSeverity, Value: "InfoU"},
				{Kind: RustTokenWhitespace, Value: " "},
				{Kind: RustTokenWord, Value: "message", NeverWildcard: true},
			},
			expectedLen:    5,
			expectedTypes:  []token.TokenType{token.TokenLocalDate, token.TokenWhitespace, token.TokenSeverityLevel, token.TokenWhitespace, token.TokenWord},
			expectedValues: []string{"yyyy-MM-dd", " ", "InfoU", " ", "message"},
		},
		{
			name: "Nested token list (flattened)",
			rustTokens: []RustToken{
				{Kind: RustTokenWord, Value: "prefix"},
				{
					Kind: RustTokenList,
					Children: []RustToken{
						{Kind: RustTokenWhitespace, Value: " "},
						{Kind: RustTokenWord, Value: "nested"},
					},
				},
			},
			expectedLen:    3,
			expectedTypes:  []token.TokenType{token.TokenWord, token.TokenWhitespace, token.TokenWord},
			expectedValues: []string{"prefix", " ", "nested"},
		},
		{
			name: "KV sequence token",
			rustTokens: []RustToken{
				{Kind: RustTokenKeyValueSequence, Value: "KV2=[key1, key2]KV"},
			},
			expectedLen:    1,
			expectedTypes:  []token.TokenType{token.TokenKeyValueSequence},
			expectedValues: []string{"KV2=[key1, key2]KV"},
		},
		{
			name: "Collapsed token preserved",
			rustTokens: []RustToken{
				{Kind: RustTokenCollapsed, Value: "*"},
			},
			expectedLen:    1,
			expectedTypes:  []token.TokenType{token.TokenCollapsedToken},
			expectedValues: []string{"*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenList := rustTokensToTokenList(tt.rustTokens)

			if tokenList.Length() != tt.expectedLen {
				t.Errorf("Expected %d tokens, got %d", tt.expectedLen, tokenList.Length())
			}

			for i, expectedType := range tt.expectedTypes {
				if i >= tokenList.Length() {
					break
				}
				if tokenList.Tokens[i].Type != expectedType {
					t.Errorf("Token %d: expected type %v, got %v", i, expectedType, tokenList.Tokens[i].Type)
				}
			}

			for i, expectedValue := range tt.expectedValues {
				if i >= tokenList.Length() {
					break
				}
				if tokenList.Tokens[i].Value != expectedValue {
					t.Errorf("Token %d: expected value %q, got %q", i, expectedValue, tokenList.Tokens[i].Value)
				}
			}
		})
	}
}

func TestRustKindToAgentType(t *testing.T) {
	tests := []struct {
		rustKind     RustTokenKind
		expectedType token.TokenType
	}{
		{RustTokenWord, token.TokenWord},
		{RustTokenWhitespace, token.TokenWhitespace},
		{RustTokenSpecialChar, token.TokenSpecialChar},
		{RustTokenNumeric, token.TokenNumeric},
		{RustTokenHTTPStatus, token.TokenHTTPStatus},
		{RustTokenHTTPMethod, token.TokenHTTPMethod},
		{RustTokenSeverity, token.TokenSeverityLevel},
		{RustTokenLocalDate, token.TokenLocalDate},
		{RustTokenLocalDateTime, token.TokenLocalDateTime},
		{RustTokenLocalTime, token.TokenLocalTime},
		{RustTokenOffsetDateTime, token.TokenOffsetDateTime},
		{RustTokenIPv4, token.TokenIPv4},
		{RustTokenIPv6, token.TokenIPv6},
		{RustTokenEmail, token.TokenEmail},
		{RustTokenRegularName, token.TokenRegularName},
		{RustTokenAbsolutePath, token.TokenAbsolutePath},
		{RustTokenURI, token.TokenURI},
		{RustTokenAuthority, token.TokenAuthority},
		{RustTokenPathWithQueryAndFragment, token.TokenPathWithQueryAndFragment},
		{RustTokenKeyValueSequence, token.TokenKeyValueSequence},
		{RustTokenCollapsed, token.TokenCollapsedToken},
		{RustTokenUnknown, token.TokenUnknown},
	}

	for _, tt := range tests {
		t.Run(string(tt.rustKind), func(t *testing.T) {
			agentType := rustKindToAgentType(tt.rustKind)
			if agentType != tt.expectedType {
				t.Errorf("Expected %v, got %v", tt.expectedType, agentType)
			}
		})
	}
}
