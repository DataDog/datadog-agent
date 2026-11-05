// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/require"
)

func TestEmptyArray(t *testing.T) {
	source := make([][]string, 0)
	result := ConcatenateTags(source...)
	require.Equal(t, []string{}, result)
}

func TestOneArray(t *testing.T) {
	source := make([][]string, 1)
	source[0] = []string{"one", "two"}
	result := ConcatenateTags(source...)
	require.Equal(t, []string{"one", "two"}, result)
}

func TestThreeArrays(t *testing.T) {
	source := make([][]string, 3)
	source[0] = []string{"one", "two"}
	source[1] = []string{}
	source[2] = []string{"4", "5", "6"}

	result := ConcatenateTags(source...)
	require.Equal(t, []string{"one", "two", "4", "5", "6"}, result)
}

func TestAppendUniqueTags(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		extraTags []string
		expected  []string
	}{
		{
			name:      "no extra tags returns the input",
			tags:      []string{"one", "two"},
			extraTags: nil,
			expected:  []string{"one", "two"},
		},
		{
			name:      "an absent tag is appended",
			tags:      []string{"one"},
			extraTags: []string{"two"},
			expected:  []string{"one", "two"},
		},
		{
			name:      "a tag already present is not appended twice",
			tags:      []string{"one", "two"},
			extraTags: []string{"two"},
			expected:  []string{"one", "two"},
		},
		{
			name:      "repeated extra tags are appended once",
			tags:      []string{"one"},
			extraTags: []string{"two", "two"},
			expected:  []string{"one", "two"},
		},
		{
			name:      "appending to an empty tagset",
			tags:      nil,
			extraTags: []string{"one"},
			expected:  []string{"one"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, AppendUniqueTags(tc.tags, tc.extraTags...))
		})
	}
}

// AppendUniqueTags must not write through to the caller's backing array, which
// matters where the input is a tagger-owned slice shared across payloads.
func TestAppendUniqueTagsDoesNotMutateInput(t *testing.T) {
	tags := make([]string, 1, 4)
	tags[0] = "one"

	result := AppendUniqueTags(tags, "two")

	require.Equal(t, []string{"one"}, tags)
	require.Equal(t, []string{"one", "two"}, result)
}
