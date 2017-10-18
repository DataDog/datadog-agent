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
		c.Add(k, v)
	}
	assert.Equal(t, len(m), c.Size())

	for k, v := range m {
		cached, err := c.Get(k)
		assert.Nil(t, err)
		assert.Equal(t, v, cached)
	}

	_, err := c.Get("notincache")
	assert.NotNil(t, err)

	items := c.Items()
	for k, v := range items {
		assert.Equal(t, m[k], v)
	}

	for k := range m {
		c.Remove(k)
	}
	assert.Equal(t, 0, c.Size())
}
