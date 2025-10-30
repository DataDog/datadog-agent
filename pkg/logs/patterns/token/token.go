// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package token provides data structures and utilities for tokenizing log messages.
package token

import (
	"fmt"
)

// TokenType represents the type of a token
type TokenType int

const (
	// Basic token types
	TokenUnknown TokenType = iota
	TokenWord
	TokenNumeric
	TokenWhitespace

	// Network-related tokens
	TokenIPv4
	TokenIPv6
	TokenEmail
	TokenURI
	TokenAbsolutePath

	// HTTP-related tokens
	TokenHttpMethod
	TokenHttpStatus

	// Log-related tokens
	TokenSeverityLevel
	TokenDate
)

// WildcardStatus describes a token's relationship to wildcards
type WildcardStatus int

const (
	// NotWildcard - This token cannot become a wildcard
	// Examples: dates, whitespace
	NotWildcard WildcardStatus = iota

	// PotentialWildcard - This token can become a wildcard
	// Examples: all words ("connection", "user123"), HTTP methods, IPs, numbers
	// Note: First word position is protected during merge
	PotentialWildcard

	// IsWildcard - This token is already a wildcard
	// Example: wildcard position in a pattern
	IsWildcard
)

// MergeResult describes the result of comparing two tokens
type MergeResult int

const (
	// Conflict - Tokens cannot merge, abort pattern creation
	// Examples: different types, generic words with different values
	Conflict MergeResult = iota

	// Identical - Tokens are the same, keep as-is
	// Examples: "Error" vs "Error", wildcard vs any value of same type
	Identical

	// Wildcard - Tokens can merge into wildcard
	// Examples: "connection" vs "replication", "user123" vs "admin456", "GET" vs "POST"
	Wildcard
)

// Token represents a single token in a log message
type Token struct {
	Type     TokenType
	Value    string
	Wildcard WildcardStatus // NotWildcard, PotentialWildcard, or IsWildcard
}

// NewToken creates a token with the specified wildcard status
func NewToken(tokenType TokenType, value string, wildcard WildcardStatus) Token {
	return Token{
		Type:     tokenType,
		Value:    value,
		Wildcard: wildcard,
	}
}

// IsHTTP returns true if the token is HTTP-related
func (t *Token) IsHTTP() bool {
	return t.Type == TokenHttpMethod || t.Type == TokenHttpStatus
}

// IsNetwork returns true if the token is network-related
func (t *Token) IsNetwork() bool {
	return t.Type == TokenIPv4 || t.Type == TokenIPv6 || t.Type == TokenEmail || t.Type == TokenURI
}

// String returns the string representation of a TokenType
func (tt TokenType) String() string {
	switch tt {
	case TokenUnknown:
		return "Unknown"
	case TokenWord:
		return "Word"
	case TokenNumeric:
		return "Numeric"
	case TokenWhitespace:
		return "Whitespace"
	case TokenIPv4:
		return "IPv4"
	case TokenIPv6:
		return "IPv6"
	case TokenEmail:
		return "Email"
	case TokenURI:
		return "URI"
	case TokenAbsolutePath:
		return "AbsolutePath"
	case TokenHttpMethod:
		return "HttpMethod"
	case TokenHttpStatus:
		return "HttpStatus"
	case TokenSeverityLevel:
		return "SeverityLevel"
	case TokenDate:
		return "Date"
	default:
		return fmt.Sprintf("TokenType(%d)", int(tt))
	}
}

// String returns a string representation of the token
func (t *Token) String() string {
	return fmt.Sprintf("%s(%s)", t.Type, t.Value)
}

// Compare checks if two tokens can merge and returns the result
func (t1 *Token) Compare(t2 *Token) MergeResult {
	// Different types cannot merge
	if t1.Type != t2.Type {
		return Conflict
	}

	// Identical value
	if t1.Value == t2.Value {
		return Identical
	}

	// t1 is wildcard - matches any value of same type
	if t1.Wildcard == IsWildcard {
		return Identical
	}

	// Different values - check if they can merge into wildcard
	// Whitespace never wildcards (structural)
	if t1.Type == TokenWhitespace {
		return Conflict
	}

	// Words only wildcard if both are PotentialWildcard
	if t1.Type == TokenWord {
		if t1.Wildcard == PotentialWildcard && t2.Wildcard == PotentialWildcard {
			return Wildcard
		}
		return Conflict
	}

	// Structured types (HTTP, IP, Numeric, Date, etc.) wildcard if same type
	// Same TokenDate type means same format structure (e.g., both RFC3339)
	return Wildcard
}
