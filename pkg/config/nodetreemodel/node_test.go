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
