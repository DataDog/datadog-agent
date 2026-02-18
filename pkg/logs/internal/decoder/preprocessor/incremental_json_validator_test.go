// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleJsonObjects(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []string
		expected []JSONState
	}{
		{
			name:     "simple key-value pair",
			inputs:   []string{`{"foo":"bar"}`},
			expected: []JSONState{Complete},
		},
		{
			name:     "simple key-value pair split",
			inputs:   []string{`{"foo":`, `"bar"`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Complete},
		},
		{
			name:     "number value",
			inputs:   []string{`{"count":`, `42`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Complete},
		},
		{
			name:     "boolean value",
			inputs:   []string{`{"enabled":`, `true`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Complete},
		},
		{
			name:     "null value",
			inputs:   []string{`{"data":`, `null`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Complete},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewIncrementalJSONValidator()
			for i, input := range tt.inputs {
				assert.Equal(t, tt.expected[i], decoder.Write([]byte(input)))
			}
		})
	}
}

func TestPrettyPrintedJson(t *testing.T) {
	jsonString := `{
    "name": "test",
    "count": 42,
    "enabled": true,
    "tags": [
        "tag1",
        "tag2"
    ]
}`

	lines := strings.Split(jsonString, "\n")
	decoder := NewIncrementalJSONValidator()
	for i, line := range lines[:len(lines)-1] {
		status := decoder.Write([]byte(line))
		assert.Equal(t, Incomplete, status, "line %d should be incomplete: %s", i, line)
	}
	assert.Equal(t, Complete, decoder.Write([]byte(lines[len(lines)-1])))
}

func TestPrettyPrintedJsonBrokenFormat(t *testing.T) {
	jsonString := `{
    "name": "test",
    "count": 42,
    "enabled": true,
    // suddenly not json`

	lines := strings.Split(jsonString, "\n")
	decoder := NewIncrementalJSONValidator()
	for i, line := range lines[:len(lines)-1] {
		status := decoder.Write([]byte(line))
		assert.Equal(t, Incomplete, status, "line %d should be incomplete: %s", i, line)
	}
	assert.Equal(t, Invalid, decoder.Write([]byte(lines[len(lines)-1])))
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []string
		expected []JSONState
	}{
		{
			name:     "empty object",
			inputs:   []string{"{", "}"},
			expected: []JSONState{Incomplete, Complete},
		},
		{
			name:     "empty array",
			inputs:   []string{`{"arr":`, `[`, `]`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Incomplete, Complete},
		},
		{
			name:     "escaped quotes in strings",
			inputs:   []string{`{"key":`, `"value \"quoted\" text"`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Complete},
		},
		{
			name:     "unicode escapes",
			inputs:   []string{`{"key":`, `"\u0041\u0042\u0043"`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Complete},
		},
		{
			name:     "whitespace heavy",
			inputs:   []string{" { ", ` "key" : `, ` "value" `, " } "},
			expected: []JSONState{Incomplete, Incomplete, Incomplete, Complete},
		},
		{
			name:     "Invalid JSON with array",
			inputs:   []string{`{`, `[`, `]`, `}`},
			expected: []JSONState{Incomplete, Invalid, Invalid, Invalid},
		},
		{
			name:     "Incomplete array",
			inputs:   []string{`{ "arr":`, `[`, `[`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Incomplete, Invalid},
		},
		{
			name:     "Incomplete nested array",
			inputs:   []string{`{ "arr":`, `[`, `[`, `]`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Incomplete, Incomplete, Invalid},
		},
		{
			name:     "Complete nested array",
			inputs:   []string{`{ "arr":`, `[`, `[`, `]`, `]`, `}`},
			expected: []JSONState{Incomplete, Incomplete, Incomplete, Incomplete, Incomplete, Complete},
		},
		{
			name:     "Standalone string",
			inputs:   []string{"hi"},
			expected: []JSONState{Invalid},
		},
		{
			name:     "Standalone string in array",
			inputs:   []string{`["hi"]`},
			expected: []JSONState{Invalid},
		},
		{
			name:     "Standalone array opening",
			inputs:   []string{`[`},
			expected: []JSONState{Invalid},
		},
		{
			name:     "Simple object followed by non-json",
			inputs:   []string{`{"foo":`, `"bar"`, `}`, `not json`},
			expected: []JSONState{Incomplete, Incomplete, Complete, Invalid},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewIncrementalJSONValidator()
			for i, input := range tt.inputs {
				assert.Equal(t, tt.expected[i], decoder.Write([]byte(input)), "on input %d: %s", i, input)
			}
		})
	}
}

func TestLargeComplexJson(t *testing.T) {
	jsonString := `{
    "id": "test-123",
    "tags": ["test", "validation"],
    "settings": {
        "enabled": true,
        "threshold": 42.5,
        "options": {
            "retry": true,
            "timeout": 30,
            "nested": {
                "very": {
                    "deep": {
                        "value": null
                    }
                }
            }
        }
    },
    "data": [
        {"type": "A", "value": 1},
        {"type": "B", "value": 2},
        {"type": "C", "value": 3}
    ],
	"nested_arrays": [
		[
			{
				"type": "A",
				"value": 1
			}
		],
		[
			{
				"type": "B",
				"value": 2
			}
		]
	]
}`

	lines := strings.Split(jsonString, "\n")
	decoder := NewIncrementalJSONValidator()
	for i, line := range lines[:len(lines)-1] {
		status := decoder.Write([]byte(line))
		assert.Equal(t, Incomplete, status, "line %d should be incomplete", i)
	}
	assert.Equal(t, Complete, decoder.Write([]byte(lines[len(lines)-1])))
}
