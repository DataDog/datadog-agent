// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	n, err := NewNode(obj)
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
