// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
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
		actualToken := tokensToString(tokens)
		assert.Equal(t, tc.expectedToken, actualToken)
	}
}

func TestTokenizerMaxCharRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("ABCDEFGHIJKLMNOP"))
	assert.Equal(t, "CCCCCCCCCC", tokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

func TestTokenizerMaxDigitRun(t *testing.T) {
	tokens, indicies := NewTokenizer(0).tokenize([]byte("0123456789012345"))
	assert.Equal(t, "DDDDDDDDDD", tokensToString(tokens))
	assert.Equal(t, []int{0}, indicies)
}

func TestAllSymbolsAreHandled(t *testing.T) {
	for i := tokens.Space; i < tokens.D1; i++ {
		str := tokenToString(i)
		assert.NotEmpty(t, str, "Token %d is not converted to a debug string", i)
		assert.NotEqual(t, getToken(byte(str[0])), tokens.C1, "Token %v is not tokenizable", str)
	}
}

func TestTokenizerHeuristic(t *testing.T) {
	tokenizer := NewTokenizer(10)
	msg := &messageContext{rawMessage: []byte("1234567890abcdefg")}
	assert.True(t, tokenizer.Process(msg))
	assert.Equal(t, "DDDDDDDDDD", tokensToString(msg.tokens), "Tokens should be limited to 10 digits")

	msg = &messageContext{rawMessage: []byte("12-12-12T12:12:12.12T12:12Z123")}
	assert.True(t, tokenizer.Process(msg))
	assert.Equal(t, "DD-DD-DDTD", tokensToString(msg.tokens), "Tokens should be limited to the first 10 bytes")
	assert.Equal(t, []int{0, 2, 3, 5, 6, 8, 9}, msg.tokenIndicies)

	msg = &messageContext{rawMessage: []byte("abc 123")}
	assert.True(t, tokenizer.Process(msg))
	assert.Equal(t, "CCC DDD", tokensToString(msg.tokens))
	assert.Equal(t, []int{0, 3, 4}, msg.tokenIndicies)

	msg = &messageContext{rawMessage: []byte("Jan 123")}
	assert.True(t, tokenizer.Process(msg))
	assert.Equal(t, "MTH DDD", tokensToString(msg.tokens))
	assert.Equal(t, []int{0, 3, 4}, msg.tokenIndicies)

	msg = &messageContext{rawMessage: []byte("123Z")}
	assert.True(t, tokenizer.Process(msg))
	assert.Equal(t, "DDDZONE", tokensToString(msg.tokens))
	assert.Equal(t, []int{0, 3}, msg.tokenIndicies)
}
