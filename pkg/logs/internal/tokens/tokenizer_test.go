// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tokens provides tokenization functionality for log messages.
package tokens

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	input         string
	expectedToken string
}

func TestTokenizer(t *testing.T) {
	testCases := []testCase{
		{input: "", expectedToken: ""},
		{input: " ", expectedToken: " "},
		{input: "a", expectedToken: "C"},
		{input: "a       b", expectedToken: "C C"},  // Spaces get truncated
		{input: "a  \t \t b", expectedToken: "C C"}, // Any spaces get truncated
		{input: "aaa", expectedToken: "CCC"},
		{input: "0", expectedToken: "D"},
		{input: "000", expectedToken: "DDD"},
		{input: "aa00", expectedToken: "CCDD"},
		{input: "abcd", expectedToken: "CCCC"},
		{input: "1234", expectedToken: "DDDD"},
		{input: "abc123", expectedToken: "CCCDDD"},
		{input: "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~", expectedToken: "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~"},
		{input: "123-abc-[foo] (bar)", expectedToken: "DDD-CCC-[CCC] (CCC)"},
		{input: "Sun Mar 2PM EST", expectedToken: "DAY MTH DPM ZONE"},
		{input: "12-12-12T12:12:12.12T12:12Z123", expectedToken: "DD-DD-DDTDD:DD:DD.DDTDD:DDZONEDDD"},
		{input: "amped", expectedToken: "CCCCC"},   // am should not be handled if it's part of a word
		{input: "am!ped", expectedToken: "PM!CCC"}, // am should be handled since it's separated by a special character
		{input: "TIME", expectedToken: "CCCC"},
		{input: "T123", expectedToken: "TDDD"},
		{input: "ZONE", expectedToken: "CCCC"},
		{input: "Z0NE", expectedToken: "ZONEDCC"},
		{input: "abc!📀🐶📊123", expectedToken: "CCC!CCCCCCCCCCDDD"},
		{input: "!!!$$$###", expectedToken: "!$#"}, // Symobl runs get truncated
	}

	tokenizer := NewTokenizer(0)
	for _, tc := range testCases {
		tokens, _ := tokenizer.tokenize([]byte(tc.input))
		actualToken := TokensToString(tokens)
		assert.Equal(t, tc.expectedToken, actualToken)
	}
}

func TestTokenizerMaxCharRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("ABCDEFGHIJKLMNOP"))
	assert.Equal(t, "CCCCCCCCCC", TokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

func TestTokenizerMaxDigitRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("0123456789012345"))
	assert.Equal(t, "DDDDDDDDDD", TokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

func TestAllSymbolsAreHandled(t *testing.T) {
	for i := Space; i < D1; i++ {
		str := tokenToString(i)
		assert.NotEmpty(t, str, "Token %d is not converted to a debug string", i)
		assert.NotEqual(t, tokenLookup[str[0]], C1, "Token %v is not tokenizable", str)
	}
}

func TestTokenizerMaxEvalBytes(t *testing.T) {
	tokenizer := NewTokenizer(10)

	toks, _ := tokenizer.Tokenize([]byte("1234567890abcdefg"))
	assert.Equal(t, "DDDDDDDDDD", TokensToString(toks), "Tokens should be limited to 10 digits")

	var indices []int
	toks, indices = tokenizer.Tokenize([]byte("12-12-12T12:12:12.12T12:12Z123"))
	assert.Equal(t, "DD-DD-DDTD", TokensToString(toks), "Tokens should be limited to the first 10 bytes")
	assert.Equal(t, []int{0, 2, 3, 5, 6, 8, 9}, indices)

	toks, indices = tokenizer.Tokenize([]byte("abc 123"))
	assert.Equal(t, "CCC DDD", TokensToString(toks))
	assert.Equal(t, []int{0, 3, 4}, indices)

	toks, indices = tokenizer.Tokenize([]byte("Jan 123"))
	assert.Equal(t, "MTH DDD", TokensToString(toks))
	assert.Equal(t, []int{0, 3, 4}, indices)

	toks, indices = tokenizer.Tokenize([]byte("123Z"))
	assert.Equal(t, "DDDZONE", TokensToString(toks))
	assert.Equal(t, []int{0, 3}, indices)
}

func TestIsMatch(t *testing.T) {
	tokenizer := NewTokenizer(0)
	// A string of 10 tokens to make math easier.
	ta, _ := tokenizer.tokenize([]byte("! @ # $ %"))
	tb, _ := tokenizer.tokenize([]byte("! @ # $ %"))

	assert.True(t, IsMatch(ta, tb, 1))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte("! @ #1a1a1"))

	assert.True(t, IsMatch(ta, tb, 0.5))
	assert.False(t, IsMatch(ta, tb, 0.55))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte("#1a1a1$ $ "))

	assert.False(t, IsMatch(ta, tb, 0.5))
	assert.True(t, IsMatch(ta, tb, 0.3))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte(""))

	assert.False(t, IsMatch(ta, tb, 0.5))
	assert.False(t, IsMatch(ta, tb, 0))
	assert.False(t, IsMatch(ta, tb, 1))

	ta, _ = tokenizer.tokenize([]byte("! @ # $ % "))
	tb, _ = tokenizer.tokenize([]byte("!"))

	assert.True(t, IsMatch(ta, tb, 1))
	assert.True(t, IsMatch(ta, tb, 0.01))
}
