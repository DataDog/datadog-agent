// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeToEmpty(t *testing.T) {
	obj := map[string]interface{}{
		"a": "apple",
		"b": 123,
		"c": map[string]interface{}{
			"d": true,
			"e": map[string]interface{}{
				"f": 456,
			},
		},
	}

	src, err := newNodeTree(obj, sourceIDFile)
	require.NoError(t, err)
	require.True(t, src.IsInnerNode())

	dst := newInnerNode(nil)

	merged, err := dst.Merge(src)
	require.NoError(t, err)

	expected := &nodeImpl{
		children: map[string]*nodeImpl{
			"a": {val: "apple", source: sourceIDFile},
			"b": {val: 123, source: sourceIDFile},
			"c": {
				children: map[string]*nodeImpl{
					"d": {val: true, source: sourceIDFile},
					"e": {
						children: map[string]*nodeImpl{
							"f": {val: 456, source: sourceIDFile},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expected, merged)
}

func TestMergeTwoTree(t *testing.T) {
	obj := map[string]interface{}{
		"a": "apple",
		"b": 123,
		"c": map[string]interface{}{
			"d": true,
			"e": map[string]interface{}{
				"f": 456,
			},
		},
	}

	obj2 := map[string]interface{}{
		"a": "orange",
		"z": 987,
		"c": map[string]interface{}{
			"d": false,
			"e": map[string]interface{}{
				"f": 456,
				"g": "kiwi",
			},
		},
	}

	base, err := newNodeTree(obj, sourceIDFile)
	require.NoError(t, err)
	require.True(t, base.IsInnerNode())

	overwrite, err := newNodeTree(obj2, sourceIDEnvVar)
	require.NoError(t, err)
	require.True(t, overwrite.IsInnerNode())

	merged, err := base.Merge(overwrite)
	require.NoError(t, err)

	expected := &nodeImpl{
		children: map[string]*nodeImpl{
			"a": {val: "orange", source: sourceIDEnvVar},
			"b": {val: 123, source: sourceIDFile},
			"z": {val: 987, source: sourceIDEnvVar},
			"c": {
				children: map[string]*nodeImpl{
					"d": {val: false, source: sourceIDEnvVar},
					"e": {
						children: map[string]*nodeImpl{
							"f": {val: 456, source: sourceIDEnvVar},
							"g": {val: "kiwi", source: sourceIDEnvVar},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expected, merged)
}

func TestMergeErrorLeafToNode(t *testing.T) {
	obj := map[string]interface{}{
		"a": "apple",
	}

	obj2 := map[string]interface{}{
		"a": map[string]interface{}{},
	}

	base, err := newNodeTree(obj, sourceIDFile)
	require.NoError(t, err)
	require.True(t, base.IsInnerNode())

	overwrite, err := newNodeTree(obj2, sourceIDEnvVar)
	require.NoError(t, err)
	require.True(t, overwrite.IsInnerNode())

	// checking leaf to node
	_, err = base.Merge(overwrite)
	require.NoError(t, err)

	// checking node to leaf
	_, err = overwrite.Merge(base)
	require.NoError(t, err)
}
