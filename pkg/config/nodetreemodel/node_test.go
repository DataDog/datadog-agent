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

func TestNewNodeAndNodeMethods(t *testing.T) {
	obj := map[string]interface{}{
		"a": "apple",
		"b": 123,
		"c": map[string]interface{}{
			"d": true,
			"e": []string{"f", "g"},
		},
	}

	n, err := NewNode(obj, model.SourceDefault)
	assert.NoError(t, err)

	keys, err := n.ChildrenKeys()
	assert.NoError(t, err)
	assert.Equal(t, keys, []string{"a", "b", "c"})

	first, err := n.GetChild("a")
	assert.NoError(t, err)

	firstLeaf := first.(LeafNode)
	str, err := firstLeaf.GetString()
	assert.NoError(t, err)
	assert.Equal(t, str, "apple")

	second, err := n.GetChild("b")
	assert.NoError(t, err)

	secondLeaf := second.(LeafNode)
	num, err := secondLeaf.GetInt()
	assert.NoError(t, err)
	assert.Equal(t, num, 123)

	third, err := n.GetChild("c")
	assert.NoError(t, err)
	_, ok := third.(LeafNode)
	assert.Equal(t, ok, false)

	keys, err = third.ChildrenKeys()
	assert.NoError(t, err)
	assert.Equal(t, keys, []string{"d", "e"})

	fourth, err := third.GetChild("d")
	assert.NoError(t, err)

	fourthLeaf := fourth.(LeafNode)
	b, err := fourthLeaf.GetBool()
	assert.NoError(t, err)
	assert.Equal(t, b, true)

	_, err = third.GetChild("e")
	assert.NoError(t, err)
}

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

	src, err := NewNode(obj, model.SourceFile)
	require.NoError(t, err)

	dst, err := newMapNodeImpl(nil, model.SourceDefault)
	require.NoError(t, err)

	err = dst.Merge(src)
	require.NoError(t, err)

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "b": "b", "c": "c"},
		val: map[string]Node{
			"a": &leafNodeImpl{val: "apple", source: model.SourceFile},
			"b": &leafNodeImpl{val: 123, source: model.SourceFile},
			"c": &innerNode{
				remapCase: map[string]string{"d": "d", "e": "e"},
				val: map[string]Node{
					"d": &leafNodeImpl{val: true, source: model.SourceFile},
					"e": &innerNode{
						remapCase: map[string]string{"f": "f"},
						val: map[string]Node{
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

	base, err := NewNode(obj, model.SourceFile)
	require.NoError(t, err)

	overwrite, err := NewNode(obj2, model.SourceEnvVar)
	require.NoError(t, err)

	err = base.(*innerNode).Merge(overwrite)
	require.NoError(t, err)

	expected := &innerNode{
		remapCase: map[string]string{"a": "a", "b": "b", "z": "z", "c": "c"},
		val: map[string]Node{
			"a": &leafNodeImpl{val: "orange", source: model.SourceEnvVar},
			"b": &leafNodeImpl{val: 123, source: model.SourceFile},
			"z": &leafNodeImpl{val: 987, source: model.SourceEnvVar},
			"c": &innerNode{
				remapCase: map[string]string{"d": "d", "e": "e"},
				val: map[string]Node{
					"d": &leafNodeImpl{val: false, source: model.SourceEnvVar},
					"e": &innerNode{
						remapCase: map[string]string{"f": "f", "g": "g"},
						val: map[string]Node{
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

	base, err := NewNode(obj, model.SourceFile)
	require.NoError(t, err)

	overwrite, err := NewNode(obj2, model.SourceEnvVar)
	require.NoError(t, err)

	// checking leaf to node
	err = base.(*innerNode).Merge(overwrite)
	require.Error(t, err)
	assert.Equal(t, "tree conflict, can't merge lead and non leaf nodes for 'a'", err.Error())

	// checking node to leaf
	err = overwrite.(*innerNode).Merge(base)
	require.Error(t, err)
	assert.Equal(t, "tree conflict, can't merge lead and non leaf nodes for 'a'", err.Error())
}

func TestMergeErrorLeaf(t *testing.T) {
	base, err := newMapNodeImpl(nil, model.SourceDefault)
	require.NoError(t, err)

	leaf, err := newLeafNodeImpl(123, model.SourceDefault)
	require.NoError(t, err)

	err = base.Merge(leaf)
	require.Error(t, err)
	assert.Equal(t, "can't merge leaf into a node", err.Error())
}
