// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicCache(t *testing.T) {
	m := map[string]interface{}{
		"a": 1,
		"b": "dos",
		"c": struct{}{},
		"d": []string{"42", "platypus"},
	}

	c := NewBasicCache()
	for k, v := range m {
		c.Add(k, v)
	}
	assert.Equal(t, len(m), c.Size())

	for k, v := range m {
		cached, found := c.Get(k)
		assert.True(t, found)
		assert.Equal(t, v, cached)
	}

	_, found := c.Get("notincache")
	assert.False(t, found)

	items := c.Items()
	for k, v := range items {
		assert.Equal(t, m[k], v)
	}

	wombat := "wombat"
	initialTimestamp := c.GetModified()
	c.modified = 0
	c.Add("d", wombat)
	cached, found := c.Get("d")
	assert.True(t, found)
	assert.Equal(t, cached, wombat)
	assert.GreaterOrEqual(t, c.GetModified(), initialTimestamp)

	for k := range m {
		c.Remove(k)
	}
	assert.Equal(t, 0, c.Size())
}
