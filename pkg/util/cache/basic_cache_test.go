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

	for k := range m {
		c.Remove(k)
	}
	assert.Equal(t, 0, c.Size())
}
