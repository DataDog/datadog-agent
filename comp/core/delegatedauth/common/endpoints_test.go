// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeListShapeEntriesJSONString(t *testing.T) {
	entries, ok := NormalizeListShapeEntries(`[{"api_key":"abc","Host":"example.com"}]`)
	require.True(t, ok)
	require.Len(t, entries, 1)
	entry, ok := entries[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", entry["api_key"])
}

func TestListEntryIdentityExcludesAPIKey(t *testing.T) {
	first, ok := ListEntryIdentity(map[string]any{"host": "logs.datadoghq.com", "api_key": "first"})
	require.True(t, ok)
	second, ok := ListEntryIdentity(map[string]any{"api_key": "second", "host": "logs.datadoghq.com"})
	require.True(t, ok)
	changedHost, ok := ListEntryIdentity(map[string]any{"host": "other.datadoghq.com", "api_key": "first"})
	require.True(t, ok)

	assert.Equal(t, first, second)
	assert.NotEqual(t, first, changedHost)
}

func TestNormalizeListShapeEntriesInvalidJSONString(t *testing.T) {
	_, ok := NormalizeListShapeEntries(`not json`)
	assert.False(t, ok)
}

func TestNormalizeListShapeEntriesSliceOfStringMap(t *testing.T) {
	entries, ok := NormalizeListShapeEntries([]any{
		map[string]any{"api_key": "abc"},
	})
	require.True(t, ok)
	require.Len(t, entries, 1)
	entry, ok := entries[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", entry["api_key"])
}

func TestNormalizeListShapeEntriesSliceOfAnyMap(t *testing.T) {
	// YAML-sourced additional_endpoints decode each entry as map[any]any, not map[string]any.
	entries, ok := NormalizeListShapeEntries([]any{
		map[any]any{"api_key": "abc", "Host": "example.com"},
	})
	require.True(t, ok)
	require.Len(t, entries, 1)
	entry, ok := entries[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", entry["api_key"])
	assert.Equal(t, "example.com", entry["Host"])
}

func TestNormalizeListShapeEntriesRegisteredDefault(t *testing.T) {
	entries, ok := NormalizeListShapeEntries([]map[string]any{
		{"api_key": "abc"},
	})
	require.True(t, ok)
	require.Len(t, entries, 1)
	entry, ok := entries[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", entry["api_key"])
}

func TestNormalizeListShapeEntriesPreservesNonMapElements(t *testing.T) {
	// A bare string or a nil (from a blank YAML list item) must round-trip unchanged rather than
	// being silently dropped - the caller rebuilds and writes back the whole list from this
	// result, so any entry lost here is deleted from the user's config.
	entries, ok := NormalizeListShapeEntries([]any{
		map[string]any{"api_key": "abc"},
		"not-a-map",
		nil,
	})
	require.True(t, ok)
	require.Len(t, entries, 3)

	entry, ok := entries[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", entry["api_key"])

	assert.Equal(t, "not-a-map", entries[1])
	assert.Nil(t, entries[2])
}

func TestNormalizeListShapeEntriesUnsupportedType(t *testing.T) {
	_, ok := NormalizeListShapeEntries(42)
	assert.False(t, ok)
}

func TestCaseInsensitiveStringField(t *testing.T) {
	entry := map[string]any{"API_KEY": "abc"}

	value, ok := CaseInsensitiveStringField(entry, "api_key")
	require.True(t, ok)
	assert.Equal(t, "abc", value)

	_, ok = CaseInsensitiveStringField(entry, "missing")
	assert.False(t, ok)
}

func TestCaseInsensitiveStringFieldWithKeyReturnsOriginalCasing(t *testing.T) {
	entry := map[string]any{"Api_Key": "abc"}

	key, value, ok := CaseInsensitiveStringFieldWithKey(entry, "api_key")
	require.True(t, ok)
	assert.Equal(t, "Api_Key", key)
	assert.Equal(t, "abc", value)
}

func TestCaseInsensitiveStringFieldWithKeyNonStringValue(t *testing.T) {
	entry := map[string]any{"api_key": 123}

	_, _, ok := CaseInsensitiveStringFieldWithKey(entry, "api_key")
	assert.False(t, ok)
}
