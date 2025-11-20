// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package token

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewToken(t *testing.T) {
	token := NewToken(TokenWord, "test", PotentialWildcard)

	assert.Equal(t, TokenWord, token.Type, "Expected TokenWord")
	assert.Equal(t, "test", token.Value, "Expected 'test'")
	assert.Equal(t, PotentialWildcard, token.Wildcard, "Expected PotentialWildcard")
}

func TestToken_Compare_DifferentTypes(t *testing.T) {
	word := NewToken(TokenWord, "hello", PotentialWildcard)
	number := NewToken(TokenNumeric, "123", PotentialWildcard)

	result := word.Compare(&number)
	assert.Equal(t, Conflict, result, "Different types should return Conflict")
}

func TestToken_Compare_SameValue(t *testing.T) {
	token1 := NewToken(TokenWord, "hello", PotentialWildcard)
	token2 := NewToken(TokenWord, "hello", PotentialWildcard)

	result := token1.Compare(&token2)
	assert.Equal(t, Identical, result, "Same values should return Identical")
}

func TestToken_Compare_WildcardMatches(t *testing.T) {
	wildcard := NewToken(TokenWord, "anything", IsWildcard)
	concrete := NewToken(TokenWord, "hello", PotentialWildcard)

	result := wildcard.Compare(&concrete)
	assert.Equal(t, Identical, result, "Wildcard should match any value of same type")
}

func TestToken_Compare_WhitespaceConflict(t *testing.T) {
	space1 := NewToken(TokenWhitespace, " ", NotWildcard)
	space2 := NewToken(TokenWhitespace, "  ", NotWildcard)

	result := space1.Compare(&space2)
	assert.Equal(t, Conflict, result, "Different whitespace should return Conflict")
}

func TestToken_Compare_WordsWithDifferentValues(t *testing.T) {
	// Both PotentialWildcard - should merge to wildcard
	word1 := NewToken(TokenWord, "hello", PotentialWildcard)
	word2 := NewToken(TokenWord, "world", PotentialWildcard)

	result := word1.Compare(&word2)
	assert.Equal(t, Wildcard, result, "Different PotentialWildcard words should return Wildcard")

	// One is NotWildcard - should conflict
	word3 := NewToken(TokenWord, "INFO", NotWildcard)
	word4 := NewToken(TokenWord, "ERROR", PotentialWildcard)

	result2 := word3.Compare(&word4)
	assert.Equal(t, Conflict, result2, "Words with NotWildcard should return Conflict")
}

func TestToken_Compare_StructuredTypes(t *testing.T) {
	// Different IPs should merge to wildcard
	ip1 := NewToken(TokenIPv4, "192.168.1.1", PotentialWildcard)
	ip2 := NewToken(TokenIPv4, "10.0.0.1", PotentialWildcard)

	result := ip1.Compare(&ip2)
	assert.Equal(t, Wildcard, result, "Different structured types (same type) should return Wildcard")

	// Different numbers should merge to wildcard
	num1 := NewToken(TokenNumeric, "123", PotentialWildcard)
	num2 := NewToken(TokenNumeric, "456", PotentialWildcard)

	result2 := num1.Compare(&num2)
	assert.Equal(t, Wildcard, result2, "Different numeric values should return Wildcard")

	// Different dates should merge to wildcard
	date1 := NewToken(TokenDate, "2023-01-01", PotentialWildcard)
	date2 := NewToken(TokenDate, "2023-12-31", PotentialWildcard)

	result3 := date1.Compare(&date2)
	assert.Equal(t, Wildcard, result3, "Different dates should return Wildcard")
}

func TestToken_String(t *testing.T) {
	// Regular token
	token := Token{Type: TokenWord, Value: "hello"}
	expected := "Word(hello)"
	assert.Equal(t, expected, token.String(), "Token String() should format correctly")

	// Wildcard token - still shows the value, not "*"
	wildcardToken := Token{Type: TokenWord, Value: "test", Wildcard: IsWildcard}
	expectedWildcard := "Word(test)"
	assert.Equal(t, expectedWildcard, wildcardToken.String(), "Wildcard token String() should show value")
}
