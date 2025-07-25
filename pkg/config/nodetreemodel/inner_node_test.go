// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
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

	node, err := NewNodeTree(obj, model.SourceFile)
	require.NoError(t, err)
	src, ok := node.(InnerNode)
	require.True(t, ok)

	dst := newInnerNode(nil)

	err = dst.Merge(src)
	require.NoError(t, err)

	expected := &innerNode{
		children: map[string]Node{
			"a": &leafNodeImpl{val: "apple", source: model.SourceFile},
			"b": &leafNodeImpl{val: 123, source: model.SourceFile},
			"c": &innerNode{
				children: map[string]Node{
					"d": &leafNodeImpl{val: true, source: model.SourceFile},
					"e": &innerNode{
						children: map[string]Node{
							"f": &leafNodeImpl{val: 456, source: model.SourceFile},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expected, dst)
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

	node, err := NewNodeTree(obj, model.SourceFile)
	require.NoError(t, err)
	base, ok := node.(InnerNode)
	require.True(t, ok)

	node, err = NewNodeTree(obj2, model.SourceEnvVar)
	require.NoError(t, err)
	overwrite, ok := node.(InnerNode)
	require.True(t, ok)

	err = base.Merge(overwrite)
	require.NoError(t, err)

	expected := &innerNode{
		children: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceEnvVar},
			"b": &leafNodeImpl{val: 123, source: model.SourceFile},
			"z": &leafNodeImpl{val: 987, source: model.SourceEnvVar},
			"c": &innerNode{
				children: map[string]Node{
					"d": &leafNodeImpl{val: false, source: model.SourceEnvVar},
					"e": &innerNode{
						children: map[string]Node{
							"f": &leafNodeImpl{val: 456, source: model.SourceEnvVar},
							"g": &leafNodeImpl{val: "kiwi", source: model.SourceEnvVar},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, expected, base)
}

func TestMergeErrorLeafToNode(t *testing.T) {
	obj := map[string]interface{}{
		"a": "apple",
	}

	obj2 := map[string]interface{}{
		"a": map[string]interface{}{},
	}

	node, err := NewNodeTree(obj, model.SourceFile)
	require.NoError(t, err)
	base, ok := node.(InnerNode)
	require.True(t, ok)

	node, err = NewNodeTree(obj2, model.SourceEnvVar)
	require.NoError(t, err)
	overwrite, ok := node.(InnerNode)
	require.True(t, ok)

	// checking leaf to node
	err = base.Merge(overwrite)
	require.NoError(t, err)

	// checking node to leaf
	err = overwrite.Merge(base)
	require.NoError(t, err)
}
