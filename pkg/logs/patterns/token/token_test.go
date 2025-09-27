// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package token

import (
	"testing"
)

func TestTokenType_String(t *testing.T) {
	tests := []struct {
		tokenType TokenType
		expected  string
	}{
		{TokenUnknown, "Unknown"},
		{TokenWord, "Word"},
		{TokenNumeric, "Numeric"},
		{TokenWhitespace, "Whitespace"},
		{TokenIPv4, "IPv4"},
		{TokenIPv6, "IPv6"},
		{TokenEmail, "Email"},
		{TokenURI, "URI"},
		{TokenAbsolutePath, "AbsolutePath"},
		{TokenHttpMethod, "HttpMethod"},
		{TokenHttpStatus, "HttpStatus"},
		{TokenSeverityLevel, "SeverityLevel"},
		{TokenDate, "Date"},
	}

	for _, test := range tests {
		result := test.tokenType.String()
		if result != test.expected {
			t.Errorf("TokenType %v: expected %s, got %s", test.tokenType, test.expected, result)
		}
	}
}

func TestToken_IsHTTP(t *testing.T) {
	httpToken := Token{Type: TokenHttpMethod, Value: "GET"}
	if !httpToken.IsHTTP() {
		t.Error("HttpMethod token should be HTTP")
	}

	statusToken := Token{Type: TokenHttpStatus, Value: "200"}
	if !statusToken.IsHTTP() {
		t.Error("HttpStatus token should be HTTP")
	}

	wordToken := Token{Type: TokenWord, Value: "test"}
	if wordToken.IsHTTP() {
		t.Error("Word token should not be HTTP")
	}
}

func TestToken_IsNetwork(t *testing.T) {
	ipv4Token := Token{Type: TokenIPv4, Value: "192.168.1.1"}
	if !ipv4Token.IsNetwork() {
		t.Error("IPv4 token should be network")
	}

	emailToken := Token{Type: TokenEmail, Value: "test@example.com"}
	if !emailToken.IsNetwork() {
		t.Error("Email token should be network")
	}

	wordToken := Token{Type: TokenWord, Value: "test"}
	if wordToken.IsNetwork() {
		t.Error("Word token should not be network")
	}
}

func TestToken_String(t *testing.T) {
	// Regular token
	token := Token{Type: TokenWord, Value: "hello"}
	expected := "Word(hello)"
	if token.String() != expected {
		t.Errorf("Expected %s, got %s", expected, token.String())
	}

	// Wildcard token
	wildcardToken := Token{Type: TokenWord, Value: "test", IsWildcard: true}
	expectedWildcard := "Word(*)"
	if wildcardToken.String() != expectedWildcard {
		t.Errorf("Expected %s, got %s", expectedWildcard, wildcardToken.String())
	}
}
