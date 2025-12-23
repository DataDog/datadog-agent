// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package converters

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeMap_BasicMerge(t *testing.T) {
	dst := yamlNode{"existing": "old", "keep": "value"}
	src := yamlNode{"existing": "new", "added": 42}

	changed := mergeMap(dst, src)

	require.True(t, changed)
	require.Equal(t, "new", dst["existing"]) // Overwritten
	require.Equal(t, "value", dst["keep"])   // Preserved
	require.Equal(t, 42, dst["added"])       // Added
}

func TestMergeMap_DeepMerge(t *testing.T) {
	dst := yamlNode{
		"level1": yamlNode{
			"level2": yamlNode{
				"keep":   "value",
				"update": "old",
			},
		},
	}
	src := yamlNode{
		"level1": yamlNode{
			"level2": yamlNode{
				"update": "new",
				"add":    "value",
			},
		},
	}

	changed := mergeMap(dst, src)

	require.True(t, changed)
	level2 := dst["level1"].(yamlNode)["level2"].(map[string]any)
	require.Equal(t, "value", level2["keep"]) // Preserved
	require.Equal(t, "new", level2["update"]) // Updated
	require.Equal(t, "value", level2["add"])  // Added
}

func TestMergeMap_EmptyMaps(t *testing.T) {
	t.Run("both empty", func(t *testing.T) {
		dst := yamlNode{}
		src := yamlNode{}
		require.False(t, mergeMap(dst, src))
		require.Empty(t, dst)
	})

	t.Run("empty source", func(t *testing.T) {
		dst := yamlNode{"key": "value"}
		src := yamlNode{}
		require.False(t, mergeMap(dst, src))
		require.Equal(t, "value", dst["key"])
	})

	t.Run("empty destination", func(t *testing.T) {
		dst := yamlNode{}
		src := yamlNode{"key": "value"}
		require.True(t, mergeMap(dst, src))
		require.Equal(t, "value", dst["key"])
	})
}

func TestMergeMap_NoChanges(t *testing.T) {
	dst := yamlNode{
		"key":    "value",
		"nested": yamlNode{"inner": "same"},
	}
	src := yamlNode{
		"key":    "value",
		"nested": yamlNode{"inner": "same"},
	}

	changed := mergeMap(dst, src)

	require.False(t, changed)
}

func TestMergeMap_TypeMismatch(t *testing.T) {
	t.Run("replace map with primitive", func(t *testing.T) {
		dst := yamlNode{"key": map[string]any{"nested": "value"}}
		src := yamlNode{"key": "primitive"}
		require.True(t, mergeMap(dst, src))
		require.Equal(t, "primitive", dst["key"])
	})

	t.Run("replace primitive with map", func(t *testing.T) {
		dst := yamlNode{"key": "primitive"}
		src := yamlNode{"key": map[string]any{"nested": "value"}}
		require.True(t, mergeMap(dst, src))
		nested, ok := dst["key"].(yamlNode)
		require.True(t, ok)
		require.Equal(t, "value", nested["nested"])
	})
}
