// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package setup

import "testing"

func TestCleanStrings(t *testing.T) {
	cleanString := "container_collect_some"

	res := FindUnexpectedUnicode(cleanString)
	if len(res) != 0 {
		t.Errorf("Expected no unexpected codepoints, but found some: %v", res)
	}
}

func TestDirtyStrings(t *testing.T) {
	dirtyString := "\u202acontainer_collect_some"

	res := FindUnexpectedUnicode(dirtyString)
	if len(res) != 1 {
		t.Errorf("Expected 1 unexpected codepoint, but found: %v", len(res))
		return
	}

	unexpected := res[0]

	if unexpected.codepoint != '\u202a' {
		t.Errorf("Did not detect bidirectional control character")
	}
}

func TestVariousCodepoints(t *testing.T) {
	tests := []struct {
		input              string
		expectedCodepoints []rune
	}{
		{
			input:              "hello world",
			expectedCodepoints: nil,
		},
		{
			input:              "hello \u202aworld",
			expectedCodepoints: []rune{'\u202a'},
		},
		{
			input:              "hello \nworld",
			expectedCodepoints: nil,
		},
		{
			input:              "hello\u00b7world\U0001f63f",
			expectedCodepoints: nil,
		},
		{
			input:              "test\u200bing",
			expectedCodepoints: []rune{'\u200b'},
		},
		{
			input:              "test\u202fte\u200asti\u2000ng",
			expectedCodepoints: []rune{'\u202f', '\u200a', '\u2000'},
		},
	}
	for _, tc := range tests {
		res := FindUnexpectedUnicode(tc.input)
		if len(res) != len(tc.expectedCodepoints) {
			t.Errorf("Expected %v unexpected codepoints but found %v: %v\n", len(tc.expectedCodepoints), len(res), res)
			continue
		}
		for i, expectedCodepoint := range tc.expectedCodepoints {
			if expectedCodepoint != res[i].codepoint {
				t.Errorf("Expected to find codepoint '%U' but instead found %v", expectedCodepoint, res[i].codepoint)
			}
		}
	}
}
