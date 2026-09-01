// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
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

	nodeTree, err := newNodeTree(obj, model.SourceDefault)
	assert.NoError(t, err)

	assert.True(t, nodeTree.IsInnerNode())

	keys := nodeTree.ChildrenKeys()
	assert.Equal(t, keys, []string{"a", "b", "c"})

	firstLeaf, err := nodeTree.GetChild("a")
	assert.NoError(t, err)

	str := firstLeaf.Get()
	assert.Equal(t, str, "apple")

	secondLeaf, err := nodeTree.GetChild("b")
	assert.NoError(t, err)

	num := secondLeaf.Get()
	assert.Equal(t, num, 123)

	thirdInner, err := nodeTree.GetChild("c")
	assert.NoError(t, err)

	keys = thirdInner.ChildrenKeys()
	assert.Equal(t, keys, []string{"d", "e"})

	fourthLeaf, err := thirdInner.GetChild("d")
	assert.NoError(t, err)

	b := fourthLeaf.Get()
	assert.Equal(t, b, true)

	fifthLeaf, err := thirdInner.GetChild("e")
	assert.NoError(t, err)

	assert.True(t, fifthLeaf.IsLeafNode())
}

func BenchmarkSetDefaultManyKeys(b *testing.B) {
	for _, n := range []int{100, 500, 1000, 2000} {
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				cfg := NewNodeTreeConfig("test", "TEST", nil)
				for k := 0; k < n; k++ {
					cfg.SetDefault(fmt.Sprintf("section%d.key%d", k%20, k), k)
				}
			}
		})
	}
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

	src, err := newNodeTree(obj, model.SourceFile)
	require.NoError(t, err)
	require.True(t, src.IsInnerNode())

	dst := newInnerNode(nil)

	merged, err := dst.Merge(src)
	require.NoError(t, err)

	expected := &nodeImpl{
		children: map[string]*nodeImpl{
			"a": {val: "apple", source: model.SourceFile},
			"b": {val: 123, source: model.SourceFile},
			"c": {
				children: map[string]*nodeImpl{
					"d": {val: true, source: model.SourceFile},
					"e": {
						children: map[string]*nodeImpl{
							"f": {val: 456, source: model.SourceFile},
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

	base, err := newNodeTree(obj, model.SourceFile)
	require.NoError(t, err)
	require.True(t, base.IsInnerNode())

	overwrite, err := newNodeTree(obj2, model.SourceEnvVar)
	require.NoError(t, err)
	require.True(t, overwrite.IsInnerNode())

	merged, err := base.Merge(overwrite)
	require.NoError(t, err)

	expected := &nodeImpl{
		children: map[string]*nodeImpl{
			"a": {val: "orange", source: model.SourceEnvVar},
			"b": {val: 123, source: model.SourceFile},
			"z": {val: 987, source: model.SourceEnvVar},
			"c": {
				children: map[string]*nodeImpl{
					"d": {val: false, source: model.SourceEnvVar},
					"e": {
						children: map[string]*nodeImpl{
							"f": {val: 456, source: model.SourceEnvVar},
							"g": {val: "kiwi", source: model.SourceEnvVar},
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

	base, err := newNodeTree(obj, model.SourceFile)
	require.NoError(t, err)
	require.True(t, base.IsInnerNode())

	overwrite, err := newNodeTree(obj2, model.SourceEnvVar)
	require.NoError(t, err)
	require.True(t, overwrite.IsInnerNode())

	// checking leaf to node
	_, err = base.Merge(overwrite)
	require.NoError(t, err)

	// checking node to leaf
	_, err = overwrite.Merge(base)
	require.NoError(t, err)
}
