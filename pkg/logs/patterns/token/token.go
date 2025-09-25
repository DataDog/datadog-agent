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

// Token represents a single token in a log message
type Token struct {
	Type       TokenType
	Value      string
	IsWildcard bool
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
