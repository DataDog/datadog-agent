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
		added := c.Add(k, v)
		assert.True(t, added)
	}
	assert.Equal(t, len(m), c.Size())

	added := c.Add("a", 1)
	assert.False(t, added) // Already there

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

	added = c.Add("d", []string{"56", "wombat"})
	assert.True(t, added) // Update there

	for k := range m {
		c.Remove(k)
	}
	assert.Equal(t, 0, c.Size())
}

func TestBasicCacheWithCustomEqFunc(t *testing.T) {
	data := map[string]interface{}{
		"1": 1,
		"2": "2",
		"3": struct{}{},
		"4": "four",
	}

	c := NewBasicCacheWithEqualityFunc(func(a, b interface{}) bool { return true })
	for k, v := range data {
		added := c.Add(k, v)
		assert.True(t, added)
	}

	assert.Equal(t, len(data), c.Size())
	m := c.GetModified()

	for k, v := range data {
		added := c.Add(k, "koala")
		assert.False(t, added)              // Already there and equal given the custom equalitfy func
		assert.Equal(t, m, c.GetModified()) // Thus the cache should not have been updated
		val, _ := c.Get(k)
		assert.Equal(t, v, val)
	}

	for k := range data {
		c.Remove(k)
	}
	assert.Equal(t, 0, c.Size())
}
