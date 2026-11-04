// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sort

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitBlankFieldsPreservesLeadingBlanks(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "single space before token",
			input:  " x",
			expect: []string{" x"},
		},
		{
			name:   "two spaces before token",
			input:  "  abc",
			expect: []string{"  abc"},
		},
		{
			name:   "no leading blanks",
			input:  "A",
			expect: []string{"A"},
		},
		{
			name:   "two fields with blanks",
			input:  "1 b",
			expect: []string{"1", " b"},
		},
		{
			name:   "multiple fields with varying blanks",
			input:  "  a   bb c",
			expect: []string{"  a", "   bb", " c"},
		},
		{
			name:   "tab as blank",
			input:  "\tx",
			expect: []string{"\tx"},
		},
		{
			name:   "trailing blanks form a field",
			input:  "a  ",
			expect: []string{"a", "  "},
		},
		{
			name:   "empty string",
			input:  "",
			expect: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitBlankFields(tt.input)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestExtractKeyCharOffsetIncludesBlanks(t *testing.T) {
	// When no -t separator is set, character offsets into a field must
	// count from the start of the field including its leading blanks.
	// This matches GNU sort behavior.
	tests := []struct {
		name   string
		line   string
		key    keySpec
		expect string
	}{
		{
			name: "offset into leading blank: ' x' k1.2,1.3 extracts 'x'",
			line: " x",
			key:  keySpec{startField: 1, startChar: 2, endField: 1, endChar: 3},
			// field 1 = " x", char 2 = 'x', char 3 = past end → "x"
			expect: "x",
		},
		{
			name: "offset lands on blank: '  abc' k1.1,1.1 extracts ' '",
			line: "  abc",
			key:  keySpec{startField: 1, startChar: 1, endField: 1, endChar: 1},
			// field 1 = "  abc", char 1 = first space
			expect: " ",
		},
		{
			name: "offset lands on second blank: '  abc' k1.2,1.2 extracts ' '",
			line: "  abc",
			key:  keySpec{startField: 1, startChar: 2, endField: 1, endChar: 2},
			// field 1 = "  abc", char 2 = second space
			expect: " ",
		},
		{
			name: "offset past blanks: '  abc' k1.3,1.3 extracts 'a'",
			line: "  abc",
			key:  keySpec{startField: 1, startChar: 3, endField: 1, endChar: 3},
			// field 1 = "  abc", char 3 = 'a'
			expect: "a",
		},
		{
			name: "second field blank preserved: '1 b' k2.1,2.1 extracts ' '",
			line: "1 b",
			key:  keySpec{startField: 2, startChar: 1, endField: 2, endChar: 1},
			// field 2 = " b", char 1 = space
			expect: " ",
		},
		{
			name: "second field offset past blank: '1 b' k2.2,2.2 extracts 'b'",
			line: "1 b",
			key:  keySpec{startField: 2, startChar: 2, endField: 2, endChar: 2},
			// field 2 = " b", char 2 = 'b'
			expect: "b",
		},
		{
			name: "end field beyond fields: 'abc' k1.2,2.1 extracts 'bc'",
			line: "abc",
			key:  keySpec{startField: 1, startChar: 2, endField: 2, endChar: 1},
			// Only 1 field; end field 2 is out of range → treat as end-of-line
			expect: "bc",
		},
		{
			name: "end field beyond fields multi-field: 'a b' k1.1,3.1 extracts 'a b'",
			line: "a b",
			key:  keySpec{startField: 1, startChar: 1, endField: 3, endChar: 1},
			// 2 blank-split fields; end field 3 out of range → end-of-line
			// position-based: returns line[0:] = "a b"
			expect: "a b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKey(tt.line, tt.key, 0, false, false, false)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestExtractKeyEndBeforeStartIsZeroWidth(t *testing.T) {
	// GNU sort treats -k 2,1 (end field < start field) as a zero-width
	// key, producing an empty string that falls back to whole-line
	// comparison during tie-breaking.
	tests := []struct {
		name   string
		line   string
		key    keySpec
		expect string
	}{
		{
			name:   "end field before start field",
			line:   "1 b",
			key:    keySpec{startField: 2, endField: 1},
			expect: "",
		},
		{
			name:   "end field well before start field",
			line:   "a bb ccc",
			key:    keySpec{startField: 3, endField: 1},
			expect: "",
		},
		{
			name:   "start field beyond line still returns empty",
			line:   "only",
			key:    keySpec{startField: 2, endField: 1},
			expect: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKey(tt.line, tt.key, 0, false, false, false)
			assert.Equal(t, tt.expect, got)
		})
	}
}
