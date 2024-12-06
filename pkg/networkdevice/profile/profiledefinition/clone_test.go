// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCopySlice(t *testing.T) {
	intSlice := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	intSliceCopy := CopySlice(intSlice)
	intSliceCopy[0] = 100
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, intSlice)
	assert.Equal(t, []int{100, 2, 3, 4, 5, 6, 7, 8, 9}, intSliceCopy)

	strSlice := []string{"a", "b", "c", "d", "e", "f", "g"}
	strSliceCopy := CopySlice(strSlice)
	strSliceCopy[0] = "aaa"
	assert.Equal(t, []string{"a", "b", "c", "d", "e", "f", "g"}, strSlice)
	assert.Equal(t, []string{"aaa", "b", "c", "d", "e", "f", "g"}, strSliceCopy)
}

func TestCopyMap(t *testing.T) {
	intMap := map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	intMapCopy := CopyMap(intMap)
	intMapCopy["a"] = 100
	assert.Equal(t, map[string]int{
		"a": 1, "b": 2, "c": 3,
	}, intMap)
	assert.Equal(t, map[string]int{
		"a": 100, "b": 2, "c": 3,
	}, intMapCopy)
}

type cloneMe struct {
	X int
	Y string
}

func (c *cloneMe) Clone() *cloneMe {
	return &cloneMe{X: c.X, Y: c.Y}
}

func TestCloneSlice(t *testing.T) {
	items := []*cloneMe{
		{1, "a"},
		{2, "b"},
		{3, "c"},
	}
	itemsCopy := CloneSlice(items)
	itemsCopy[0].X = 100
	itemsCopy[1] = &cloneMe{200, "bbb"}
	assert.Equal(t, []*cloneMe{
		{1, "a"},
		{2, "b"},
		{3, "c"},
	}, items)
	assert.Equal(t, []*cloneMe{
		{100, "a"},
		{200, "bbb"},
		{3, "c"},
	}, itemsCopy)
}

func TestCloneMap(t *testing.T) {
	m := map[string]*cloneMe{
		"Item A": {1, "a"},
		"Item B": {2, "b"},
	}
	mCopy := CloneMap(m)
	mCopy["Item A"].X = 100
	mCopy["Item B"] = &cloneMe{200, "bbb"}
	mCopy["Item C"] = &cloneMe{300, "ccc"}
	assert.Equal(t, map[string]*cloneMe{
		"Item A": {1, "a"},
		"Item B": {2, "b"},
	}, m)
	assert.Equal(t, map[string]*cloneMe{
		"Item A": {100, "a"},
		"Item B": {200, "bbb"},
		"Item C": {300, "ccc"},
	}, mCopy)
}
