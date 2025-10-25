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

// MergeabilityLevel represents how two tokens can be merged
type MergeabilityLevel int

// In Progress
const (
	Unmergeable MergeabilityLevel = iota
	MergeableAsNewType
	MergeableAsWildcard
	MergeableWithWiderRange
	Fits
	FitsAsItIs
)

// IsMergeable returns true if the mergeability level allows merging
func (m MergeabilityLevel) IsMergeable() bool {
	return m > Unmergeable
}

// Compare returns the comparison result with another mergeability level
func (m1 MergeabilityLevel) Compare(m2 MergeabilityLevel) int {
	return int(m1) - int(m2)
}

// Token represents a single token in a log message
type Token struct {
	Type             TokenType
	Value            string
	IsWildcard       bool
	PossiblyWildcard bool // Indicates if this token can merge into a wildcard during batch consolidation
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
	if t.IsWildcard {
		return fmt.Sprintf("%s(*)", t.Type)
	}
	return fmt.Sprintf("%s(%s)", t.Type, t.Value)
}

// In Progress: GetMergeabilityLevel determines how this token can merge with another token
func (t1 *Token) GetMergeabilityLevel(t2 *Token) MergeabilityLevel {
	// Same token type and value
	if t1.Type == t2.Type && t1.Value == t2.Value {
		return FitsAsItIs
	}

	// Same token type but different values
	if t1.Type == t2.Type {
		// Check if both can be wildcards
		if t1.PossiblyWildcard && t2.PossiblyWildcard {
			return MergeableAsWildcard
		}
		// Generic words don't merge
		return Unmergeable
	}

	// Different token types
	return Unmergeable
}

// In Progress: MergeWith creates a merged token from this token and another
func (t1 *Token) MergeWith(other *Token) *Token {
	level := t1.GetMergeabilityLevel(other)

	switch level {
	case FitsAsItIs:
		return t1 // Return this token unchanged
	case MergeableAsWildcard:
		// Create a wildcard token
		return &Token{
			Type:             t1.Type,
			Value:            "*",
			IsWildcard:       true,
			PossiblyWildcard: true,
		}
	default:
		return t1 // Return this token unchanged
	}
}
