package config

import "testing"

func TestCleanStrings(t *testing.T) {
	cleanString := "container_collect_some"

	res := FreeFromUnexpectedUnicode([]byte(cleanString))
	if len(res) != 0 {
		t.Errorf("Expected no unexpected codepoints, but found some: %v", res)
	}
}

func TestDirtyStrings(t *testing.T) {
	dirtyString := "‪container_collect_some"

	res := FreeFromUnexpectedUnicode([]byte(dirtyString))
	if len(res) != 1 {
		t.Errorf("Expected 1 unexpected codepoint, but found: %v", len(res))
		return
	}

	unexpected := res[0]

	if unexpected.codepoint != '‪' {
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
			input:              []byte("hello ‪world"),
			expectedCodepoints: []rune{'‪'},
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
			input:              []byte("test​ing"),
			expectedCodepoints: []rune{'​'},
		},
		{
			input:              []byte("test te sti ng"),
			expectedCodepoints: []rune{' ', ' ', ' '},
		},
	}
	for _, tc := range tests {
		res := FreeFromUnexpectedUnicode(tc.input)
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
