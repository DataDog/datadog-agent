// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package token provides data structures and utilities for tokenizing log messages.
package token

import (
	"strings"
)

// TokenList represents a sequence of tokens
type TokenList struct {
	Tokens []Token
}

// NewTokenList creates a new empty TokenList
func NewTokenList() *TokenList {
	return &TokenList{Tokens: make([]Token, 0)}
}

// NewTokenListWithTokens creates a new TokenList with the provided tokens
func NewTokenListWithTokens(tokens []Token) *TokenList {
	return &TokenList{Tokens: tokens}
}

// Add appends one or more tokens to the list
func (tl *TokenList) Add(tokens ...Token) {
	tl.Tokens = append(tl.Tokens, tokens...)
}

// Length returns the number of tokens
func (tl *TokenList) Length() int {
	return len(tl.Tokens)
}

// IsEmpty returns true if the list is empty
func (tl *TokenList) IsEmpty() bool {
	return len(tl.Tokens) == 0
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
