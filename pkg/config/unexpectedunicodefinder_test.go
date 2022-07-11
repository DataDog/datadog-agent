package config

import "testing"

func TestCleanStrings(t *testing.T) {
	cleanString := "container_collect_some"

	res := FindUnexpectedUnicode([]byte(cleanString))
	if len(res) != 0 {
		t.Errorf("Expected no unexpected codepoints, but found some: %v", res)
	}
}

func TestDirtyStrings(t *testing.T) {
	dirtyString := "\u202acontainer_collect_some"

	res := FindUnexpectedUnicode([]byte(dirtyString))
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
		input              []byte
		expectedCodepoints []rune
	}{
		{
			input:              []byte("hello world"),
			expectedCodepoints: nil,
		},
		{
			input:              []byte("hello \u202aworld"),
			expectedCodepoints: []rune{'\u202a'},
		},
		{
			input:              []byte("hello \nworld"),
			expectedCodepoints: nil,
		},
		{
			input:              []byte("hello·world😿"),
			expectedCodepoints: nil,
		},
		{
			input:              []byte("test\u200bing"),
			expectedCodepoints: []rune{'\u200b'},
		},
		{
			input:              []byte("test\u202fte\u200asti\u2000ng"),
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
