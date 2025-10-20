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

// DateComponents represents parsed components of a date token
type DateComponents struct {
	Year   string
	Month  string
	Day    string
	Hour   string
	Minute string
	Second string
	Format string // Original format pattern
}

// Token represents a single token in a log message
type Token struct {
	Type             TokenType
	Value            string
	IsWildcard       bool
	PossiblyWildcard bool // Indicates if this token can merge into a wildcard during batch consolidation

	// Advanced token structure information
	DateInfo *DateComponents // For TokenDate - parsed date components
}

// NewToken creates a new token with the given type and value
func NewToken(tokenType TokenType, value string) Token {
	return Token{
		Type:             tokenType,
		Value:            value,
		IsWildcard:       false,
		PossiblyWildcard: false,
	}
}

// NewTokenWithFlags creates a new token with explicit wildcard flags
func NewTokenWithFlags(tokenType TokenType, value string, isWildcard, possiblyWildcard bool) Token {
	return Token{
		Type:             tokenType,
		Value:            value,
		IsWildcard:       isWildcard,
		PossiblyWildcard: possiblyWildcard,
	}
}

// NewWildcardToken creates a wildcard token of the given type
func NewWildcardToken(tokenType TokenType) Token {
	return Token{
		Type:             tokenType,
		Value:            "*",
		IsWildcard:       true,
		PossiblyWildcard: true,
	}
}

// NewPossiblyWildcardToken creates a token that can potentially become a wildcard
func NewPossiblyWildcardToken(tokenType TokenType, value string) Token {
	return Token{
		Type:             tokenType,
		Value:            value,
		IsWildcard:       false,
		PossiblyWildcard: true,
	}
}

// NewDateToken creates a date token with parsed components
func NewDateToken(value string, dateInfo *DateComponents) Token {
	return Token{
		Type:             TokenDate,
		Value:            value,
		IsWildcard:       false,
		PossiblyWildcard: false,
		DateInfo:         dateInfo,
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
	if t.IsWildcard {
		return fmt.Sprintf("%s(*)", t.Type)
	}
	return fmt.Sprintf("%s(%s)", t.Type, t.Value)
}

// GetMergeabilityLevel determines how this token can merge with another token
func (t1 *Token) GetMergeabilityLevel(t2 *Token) MergeabilityLevel {
	// Same token type and value
	if t1.Type == t2.Type && t1.Value == t2.Value {
		return FitsAsItIs
	}

	// Same token type but different values
	if t1.Type == t2.Type {
		// Special handling for structured date tokens
		if t1.Type == TokenDate && t1.DateInfo != nil && t2.DateInfo != nil {
			return getDateMergeabilityLevel(t1.DateInfo, t2.DateInfo)
		}

		// For Word tokens, only merge if both have possiblyWildcard flag
		// This prevents generic words like "bob" and "cat" from merging
		if t1.Type == TokenWord {
			if t1.PossiblyWildcard && t2.PossiblyWildcard {
				return MergeableAsWildcard
			}
			// Generic words without numeric patterns don't merge
			return Unmergeable
		}

		// For non-Word tokens (HttpMethod, HttpStatus, AbsolutePath, Numeric, etc.)
		// they are mergeable by default since they represent structured data
		// e.g., "GET" vs "POST", "/api" vs "/users", "200" vs "404"
		return MergeableAsWildcard
	}

	// Different token types
	return Unmergeable
}

// getDateMergeabilityLevel determines how two date tokens can merge based on their structure
func getDateMergeabilityLevel(d1, d2 *DateComponents) MergeabilityLevel {
	// Must have same format to be mergeable - different formats = different log sources
	if d1.Format != d2.Format {
		return Unmergeable
	}

	// Simple rule: Only merge if same date, different time (same log source over time)
	// Everything else is likely different log sources and shouldn't merge
	sameDate := d1.Year == d2.Year && d1.Month == d2.Month && d1.Day == d2.Day
	sameTime := d1.Hour == d2.Hour && d1.Minute == d2.Minute && d1.Second == d2.Second

	if sameDate && sameTime {
		return FitsAsItIs
	}

	if sameDate && !sameTime {
		// Same date, different time = same log source at different times
		return MergeableWithWiderRange
	}

	// Different dates = different log sources/periods = don't merge
	return Unmergeable
}

// NOTE: MergeWith() and createPartialDateWildcard() have been moved to the
// clustering/merging package. Token now only provides data comparison via
// GetMergeabilityLevel(), while merge execution is handled as business logic
// in the merging package.
