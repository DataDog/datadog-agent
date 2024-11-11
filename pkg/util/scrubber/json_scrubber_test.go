// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScrubJSON(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "simple",
			input:    []byte(`{"password": "secret", "username": "user1"}`),
			expected: []byte(`{"password": "********", "username": "user1"}`),
		},
		{
			name:     "No sensitive info to scrub",
			input:    []byte(`{"message": "hello world", "count": 123}`),
			expected: []byte(`{"message": "hello world", "count": 123}`),
		},
		{
			name:     "nested",
			input:    []byte(`{"user": {"password": "secret", "email": "user@example.com"}}`),
			expected: []byte(`{"user": {"password": "********", "email": "user@example.com"}}`),
		},
		{
			name:     "array",
			input:    []byte(`[{"password": "secret1"}, {"password": "secret2"}]`),
			expected: []byte(`[{"password": "********"}, {"password": "********"}]`),
		},
		{
			name:     "empty object",
			input:    []byte(`{}`),
			expected: []byte(`{}`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := ScrubJSON(tc.input)
			require.NoError(t, err)

			require.JSONEq(t, string(tc.expected), string(actual))
		})
	}

	t.Run("malformed", func(t *testing.T) {
		scrubber := New()
		scrubber.AddReplacer(SingleLine, Replacer{
			Regex: regexp.MustCompile("foo"),
			Repl:  []byte("bar"),
		})

		input := `{"foo": "bar", "baz"}`
		expected := `{"bar": "bar", "baz"}`
		actual, err := scrubber.ScrubJSON([]byte(input))
		require.NoError(t, err)
		require.Equal(t, expected, string(actual))
	})
}
