// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build rust_preprocessor && cgo

package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testParityCases are shared between Go and Rust tokenizer tests.
var testParityCases = []struct {
	input    string
	expected string
}{
	{"", ""},
	{" ", " "},
	{"a", "C"},
	{"a       b", "C C"},
	{"a  \t \t b", "C C"},
	{"aaa", "CCC"},
	{"0", "D"},
	{"000", "DDD"},
	{"aa00", "CCDD"},
	{"abcd", "CCCC"},
	{"1234", "DDDD"},
	{"abc123", "CCCDDD"},
	{"!@#$%^&*()_+[]:-/\\.,\\'{}\"`~", "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~"},
	{"123-abc-[foo] (bar)", "DDD-CCC-[CCC] (CCC)"},
	{"Sun Mar 2PM EST", "DAY MTH DPM ZONE"},
	{"12-12-12T12:12:12.12T12:12Z123", "DD-DD-DDTDD:DD:DD.DDTDD:DDZONEDDD"},
	{"amped", "CCCCC"},
	{"am!ped", "PM!CCC"},
	{"TIME", "CCCC"},
	{"T123", "TDDD"},
	{"ZONE", "CCCC"},
	{"Z0NE", "ZONEDCC"},
	{"abc!\U0001f4c0\U0001f436\U0001f4ca123", "CCC!CCCCCCCCCCDDD"},
	{"!!!$$$###", "!$#"},
	{"FATAL", "FATAL"},
	{"fatal", "FATAL"},
	{"Fatal", "FATAL"},
	{"ERROR", "ERROR"},
	{"PANIC", "PANIC"},
	{"ALERT", "ALERT"},
	{"SEVERE", "SEVERE"},
	{"WARN", "WARN"},
	{"WARNING", "WARN"},
	{"CRIT", "CRIT"},
	{"CRITICAL", "CRIT"},
	{"EMERG", "EMERG"},
	{"EMERGENCY", "EMERG"},
	{"EXCEPTION", "EXCEPTION"},
	{"CRASH", "CRASH"},
	{"CRASHED", "CRASH"},
	{"FAILED", "FAILURE"},
	{"FAILURE", "FAILURE"},
	{"DEADLOCK", "DEADLOCK"},
	{"TIMEOUT", "TIMEOUT"},
	{"EXCEPTIONS", "CCCCCCCCCC"},
	{"FATALIZER", "CCCCCCCCC"},
	{"[ERROR] something", "[ERROR] CCCCCCCCC"},
	{"FATAL: disk full", "FATAL: CCCC CCCC"},
}

func TestRustTokenizerParity(t *testing.T) {
	goTok := NewTokenizer(0)
	rustTok := NewRustTokenizer(0)
	defer rustTok.Close()

	for _, tc := range testParityCases {
		t.Run(tc.input, func(t *testing.T) {
			goTokens, goIdx := goTok.Tokenize([]byte(tc.input))
			rustTokens, rustIdx := rustTok.Tokenize([]byte(tc.input))
			assert.Equal(t, goTokens, rustTokens, "token mismatch")
			assert.Equal(t, goIdx, rustIdx, "index mismatch")
		})
	}
}

func TestRustTokenizerMaxEvalBytes(t *testing.T) {
	goTok := NewTokenizer(10)
	rustTok := NewRustTokenizer(10)
	defer rustTok.Close()

	input := []byte("12-12-12T12:12:12.12T12:12Z123")
	goTokens, goIdx := goTok.Tokenize(input)
	rustTokens, rustIdx := rustTok.Tokenize(input)
	assert.Equal(t, goTokens, rustTokens)
	assert.Equal(t, goIdx, rustIdx)
}

func TestRustTokenizerProduction(t *testing.T) {
	goTok := NewTokenizer(0)
	rustTok := NewRustTokenizer(0)
	defer rustTok.Close()

	for _, line := range loadBenchCorpus {
		goTokens, goIdx := goTok.Tokenize(line)
		rustTokens, rustIdx := rustTok.Tokenize(line)
		assert.Equal(t, goTokens, rustTokens, "token mismatch on: %s", string(line[:min(60, len(line))]))
		assert.Equal(t, goIdx, rustIdx, "index mismatch on: %s", string(line[:min(60, len(line))]))
	}
}

func FuzzRustTokenizerParity(f *testing.F) {
	f.Add([]byte("2024-01-15 10:30:45 INFO request processed id=123"))
	f.Add([]byte(""))
	f.Add([]byte("!!!$$$###"))
	f.Add([]byte("Jan Mon UTC PST CEST"))
	f.Add([]byte("T Z am PM"))
	f.Add([]byte("abc!\U0001f4c0\U0001f436\U0001f4ca123"))
	f.Add([]byte("Sun Mar 2PM EST JAN FEB MAR"))
	f.Add([]byte("12-12-12T12:12:12.12T12:12Z123"))
	f.Fuzz(func(t *testing.T, input []byte) {
		goTok := NewTokenizer(0)
		rustTok := NewRustTokenizer(0)
		defer rustTok.Close()
		goTokens, goIdx := goTok.Tokenize(input)
		rustTokens, rustIdx := rustTok.Tokenize(input)
		assert.Equal(t, goTokens, rustTokens)
		assert.Equal(t, goIdx, rustIdx)
	})
}
