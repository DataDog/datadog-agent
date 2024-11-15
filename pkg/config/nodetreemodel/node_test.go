// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
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

	node, err := NewNodeTree(obj, model.SourceDefault)
	assert.NoError(t, err)

	n, ok := node.(InnerNode)
	assert.True(t, ok)

	keys := n.ChildrenKeys()
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

	child, err := n.GetChild("c")
	assert.NoError(t, err)
	third := child.(InnerNode)

	keys = third.ChildrenKeys()
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
