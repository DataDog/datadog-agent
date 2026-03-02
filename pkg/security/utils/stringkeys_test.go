// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/mailru/easyjson/jwriter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStringKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "create from slice",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "deduplicate",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single element",
			input:    []string{"single"},
			expected: []string{"single"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sk := NewStringKeys(tt.input)
			keys := sk.Keys()
			sort.Strings(keys)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, keys)
		})
	}
}

func TestStringKeys_Insert(t *testing.T) {
	sk := NewStringKeys([]string{"a"})
	sk.Insert("b")
	sk.Insert("c")
	sk.Insert("a") // duplicate

	keys := sk.Keys()
	sort.Strings(keys)
	assert.Equal(t, []string{"a", "b", "c"}, keys)
}

func TestStringKeys_ForEach(t *testing.T) {
	sk := NewStringKeys([]string{"a", "b", "c"})
	var collected []string

	sk.ForEach(func(s string) {
		collected = append(collected, s)
	})

	sort.Strings(collected)
	assert.Equal(t, []string{"a", "b", "c"}, collected)
}

func TestStringKeys_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input []string
	}{
		{
			name:  "multiple keys",
			input: []string{"a", "b", "c"},
		},
		{
			name:  "empty",
			input: []string{},
		},
		{
			name:  "single key",
			input: []string{"single"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sk := NewStringKeys(tt.input)
			data, err := sk.MarshalJSON()
			require.NoError(t, err)

			var result []string
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			sort.Strings(result)
			sort.Strings(tt.input)
			assert.Equal(t, tt.input, result)
		})
	}
}

func TestStringKeys_MarshalEasyJSON(t *testing.T) {
	t.Run("with values", func(t *testing.T) {
		sk := NewStringKeys([]string{"a", "b"})
		w := &jwriter.Writer{}
		sk.MarshalEasyJSON(w)
		data, err := w.BuildBytes()
		require.NoError(t, err)

		var result []string
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)
		sort.Strings(result)
		assert.Equal(t, []string{"a", "b"}, result)
	})

	t.Run("empty without NilSliceAsEmpty flag", func(t *testing.T) {
		sk := NewStringKeys([]string{})
		w := &jwriter.Writer{}
		sk.MarshalEasyJSON(w)
		data, err := w.BuildBytes()
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("empty with NilSliceAsEmpty flag", func(t *testing.T) {
		sk := NewStringKeys([]string{})
		w := &jwriter.Writer{
			Flags: jwriter.NilSliceAsEmpty,
		}
		sk.MarshalEasyJSON(w)
		data, err := w.BuildBytes()
		require.NoError(t, err)
		assert.Equal(t, "[]", string(data))
	})
}
