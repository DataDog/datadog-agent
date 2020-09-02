package obfuscate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryCache(t *testing.T) {
	oq1 := &ObfuscatedQuery{Query: "query1", TablesCSV: "a,b"}
	oq2 := &ObfuscatedQuery{Query: "query11", TablesCSV: "c,d,e"}
	oq1size := uint64(len("k1") + len(oq1.Query) + len(oq1.TablesCSV))
	oq2size := uint64(len("k11") + len(oq2.Query) + len(oq2.TablesCSV))

	t.Run("constructor", func(t *testing.T) {
		c := newQueryCache(5)
		assert.Equal(t, uint64(5), c.maxSize)
		assert.NotNil(t, c.items)
		assert.NotNil(t, c.list)
	})

	t.Run("Add", func(t *testing.T) {
		t.Run("one", func(t *testing.T) {
			c := newQueryCache(500)

			c.Add("k1", oq1)

			assert.Len(t, c.items, 1)
			assert.Equal(t, 1, c.list.Len())
			assert.Equal(t, oq1, c.list.Front().Value.(*cacheItem).val)
			assert.Equal(t, oq1size, c.size)
		})

		t.Run("multiple", func(t *testing.T) {
			c := newQueryCache(500)

			c.Add("k1", oq1)
			c.Add("k11", oq2)

			assert.Len(t, c.items, 2)
			assert.Equal(t, 2, c.list.Len())
			assert.Equal(t, oq1size+oq2size, c.size)

			// ensure order
			assert.Equal(t, oq2, c.list.Front().Value.(*cacheItem).val)
			assert.Equal(t, oq1, c.list.Front().Next().Value.(*cacheItem).val)
		})

		t.Run("overflow", func(t *testing.T) {
			c := newQueryCache(oq1size + oq2size)

			c.Add("k1", oq1)
			c.Add("k2", oq2)
			c.Add("k3", oq1) // overflow

			assert.Len(t, c.items, 2)
			assert.Equal(t, 2, c.list.Len())
			assert.Equal(t, "k3", c.list.Front().Value.(*cacheItem).key)
			assert.Equal(t, oq1, c.list.Front().Value.(*cacheItem).val)
			assert.Equal(t, "k2", c.list.Front().Next().Value.(*cacheItem).key)
			assert.Equal(t, oq2, c.list.Front().Next().Value.(*cacheItem).val)
		})

		t.Run("exists", func(t *testing.T) {
			c := newQueryCache(500)

			c.Add("k1", oq1)
			c.Add("k2", oq1)
			assert.Equal(t, "k2", c.list.Front().Value.(*cacheItem).key)

			c.Add("k1", oq1)
			assert.Equal(t, "k1", c.list.Front().Value.(*cacheItem).key)
		})
	})

	t.Run("Get", func(t *testing.T) {
		c := newQueryCache(500)
		c.Add("k1", oq1)
		c.Add("k2", oq2)

		t.Run("miss", func(t *testing.T) {
			v, ok := c.Get("k3")
			assert.Nil(t, v)
			assert.False(t, ok)
			assert.EqualValues(t, 1, c.misses)
		})

		t.Run("hit", func(t *testing.T) {
			v, ok := c.Get("k1")
			assert.Equal(t, oq1, v)
			assert.True(t, ok)
			assert.EqualValues(t, 1, c.hits)
			// ensure that k1 was moved to front
			assert.Equal(t, oq1, c.list.Front().Value.(*cacheItem).val)
		})
	})
}
