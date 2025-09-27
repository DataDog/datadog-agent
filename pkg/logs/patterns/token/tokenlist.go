// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package token provides data structures and utilities for tokenizing log messages.
package token

import (
	"fmt"
	"sort"
	"strings"
)

// TokenList represents a sequence of tokens
type TokenList struct {
	Tokens []Token
}

// NewTokenList creates a new TokenList
func NewTokenList(tokens ...[]Token) *TokenList {
	tl := &TokenList{
		Tokens: make([]Token, 0),
	}
	if len(tokens) > 0 {
		tl.Tokens = tokens[0]
	}
	return tl
}

// Add appends a token to the list
func (tl *TokenList) Add(token Token) {
	tl.Tokens = append(tl.Tokens, token)
}

// Length returns the number of tokens
func (tl *TokenList) Length() int {
	return len(tl.Tokens)
}

// IsEmpty returns true if the list is empty
func (tl *TokenList) IsEmpty() bool {
	return len(tl.Tokens) == 0
}

// Signature generates a signature for this TokenList
func (tl *TokenList) Signature() Signature {
	return NewSignature(tl)
}

// String returns a string representation
func (tl *TokenList) String() string {
	if tl.IsEmpty() {
		return "[]"
	}

	var parts []string
	for _, token := range tl.Tokens {
		parts = append(parts, token.String())
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// PositionSignature generates position-based signature (sequence of types)
func (tl *TokenList) PositionSignature() string {
	if tl.IsEmpty() {
		return ""
	}

	var positionParts []string
	for _, token := range tl.Tokens {
		positionParts = append(positionParts, token.Type.String())
	}
	return strings.Join(positionParts, "|")
}

// CountSignature generates count-based signature (type frequencies)
func (tl *TokenList) CountSignature() string {
	if tl.IsEmpty() {
		return ""
	}

	typeCounts := make(map[TokenType]int)
	for _, token := range tl.Tokens {
		typeCounts[token.Type]++
	}

	var countParts []string
	for tokenType, count := range typeCounts {
		countParts = append(countParts, fmt.Sprintf("%s:%d", tokenType.String(), count))
	}

	// Sort to ensure deterministic signature
	sort.Strings(countParts)
	return strings.Join(countParts, ";")
}
