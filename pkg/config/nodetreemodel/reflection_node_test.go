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

type Object struct {
	Name string
	Num  int
}

func TestNewReflectionNode(t *testing.T) {
	node, err := asReflectionNode(Object{
		Name: "test",
		Num:  7,
	})
	assert.NoError(t, err)

	n, ok := node.(InnerNode)
	require.True(t, ok)

	keys := n.ChildrenKeys()
	assert.Equal(t, keys, []string{"name", "num"})

	first, err := n.GetChild("name")
	assert.NoError(t, err)

	firstLeaf := first.(LeafNode)
	str, err := firstLeaf.GetString()
	assert.NoError(t, err)
	assert.Equal(t, str, "test")

	second, err := n.GetChild("num")
	assert.NoError(t, err)

	secondLeaf := second.(LeafNode)
	num, err := secondLeaf.GetInt()
	assert.NoError(t, err)
	assert.Equal(t, num, 7)
}
