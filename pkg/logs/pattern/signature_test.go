// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pattern

import "testing"

func TestSignatureMatchesExpectedTokenStrings(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{input: "", expected: ""},
		{input: " ", expected: " "},
		{input: "a", expected: "C"},
		{input: "a       b", expected: "C C"},  // spaces get truncated
		{input: "a  \t \t b", expected: "C C"}, // any spaces get truncated
		{input: "aaa", expected: "CCC"},
		{input: "0", expected: "D"},
		{input: "000", expected: "DDD"},
		{input: "aa00", expected: "CCDD"},
		{input: "abcd", expected: "CCCC"},
		{input: "1234", expected: "DDDD"},
		{input: "abc123", expected: "CCCDDD"},
		{input: "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~", expected: "!@#$%^&*()_+[]:-/\\.,\\'{}\"`~"},
		{input: "123-abc-[foo] (bar)", expected: "DDD-CCC-[CCC] (CCC)"},
		{input: "Sun Mar 2PM EST", expected: "DAY MTH DPM ZONE"},
		{input: "12-12-12T12:12:12.12T12:12Z123", expected: "DD-DD-DDTDD:DD:DD.DDTDD:DDZONEDDD"},
		{input: "amped", expected: "CCCCC"},   // am should not be handled if it's part of a word
		{input: "am!ped", expected: "PM!CCC"}, // am should be handled since it's separated by a special character
		{input: "TIME", expected: "CCCC"},
		{input: "T123", expected: "TDDD"},
		{input: "ZONE", expected: "CCCC"},
		{input: "Z0NE", expected: "ZONEDCC"},
		{input: "abc!📀🐶📊123", expected: "CCC!CCCCCCCCCCDDD"},
		{input: "!!!$$$###", expected: "!$#"}, // symbol runs get truncated
	}

	for _, tc := range testCases {
		if got := Signature([]byte(tc.input), 0); got != tc.expected {
			t.Fatalf("Signature(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestSignatureMaxEvalBytes(t *testing.T) {
	// Mirrors the existing tokenizer heuristic truncation behavior.
	input := []byte("12-12-12T12:12:12.12T12:12Z123")
	if got := Signature(input, 10); got != "DD-DD-DDTD" {
		t.Fatalf("Signature(maxEvalBytes=10) = %q, expected %q", got, "DD-DD-DDTD")
	}
}
