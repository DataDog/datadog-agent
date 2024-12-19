// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profiledefinition

import (
	"github.com/stretchr/testify/assert"
	"slices"
	"testing"
)

type cloneMe struct {
	label *string
	ps    []int
}

func Item(label string, ps ...int) *cloneMe {
	return &cloneMe{
		label: &label,
		ps:    ps,
	}
}

func (c *cloneMe) Clone() *cloneMe {
	c2 := &cloneMe{
		ps: slices.Clone(c.ps),
	}
	if c.label != nil {
		var tmp string = *c.label
		c2.label = &tmp
	}
	return c2
}

func TestCloneSlice(t *testing.T) {
	items := []*cloneMe{
		Item("a", 1, 2, 3, 4),
		Item("b", 1, 2),
	}
	itemsCopy := CloneSlice(items)
	*itemsCopy[0].label = "aaa"
	itemsCopy[1] = Item("bbb", 10, 20)
	itemsCopy = append(itemsCopy, Item("ccc", 100, 200))
	// items is unchanged
	assert.Equal(t, []*cloneMe{
		Item("a", 1, 2, 3, 4),
		Item("b", 1, 2),
	}, items)
	assert.Equal(t, []*cloneMe{
		Item("aaa", 1, 2, 3, 4),
		Item("bbb", 10, 20),
		Item("ccc", 100, 200),
	}, itemsCopy)
}

func TestCloneMap(t *testing.T) {
	m := map[string]*cloneMe{
		"Item A": Item("a", 1, 2, 3, 4),
		"Item B": Item("b", 1, 2),
	}
	mCopy := CloneMap(m)
	mCopy["Item A"].ps[0] = 100
	mCopy["Item B"] = Item("bbb", 10, 20)
	mCopy["Item C"] = Item("ccc", 100, 200)
	assert.Equal(t, map[string]*cloneMe{
		"Item A": Item("a", 1, 2, 3, 4),
		"Item B": Item("b", 1, 2),
	}, m)
	assert.Equal(t, map[string]*cloneMe{
		"Item A": Item("a", 100, 2, 3, 4),
		"Item B": Item("bbb", 10, 20),
		"Item C": Item("ccc", 100, 200),
	}, mCopy)
}
