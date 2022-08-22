package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMetric(t *testing.T) {
	assert := assert.New(t)

	t.Run("different names", func(t *testing.T) {
		Clear()
		m1 := NewMetric("m1")
		m2 := NewMetric("m2")

		assert.NotEqual(m1, m2)
	})

	t.Run("same name", func(t *testing.T) {
		Clear()
		m1 := NewMetric("foo")
		m2 := NewMetric("foo")

		// ensure that if try to create the same metric
		// with the same name, we get back the existing
		// instance instead of creating a new one
		assert.Equal(m1, m2)
	})

	t.Run("same name and different tags", func(t *testing.T) {
		Clear()
		m1 := NewMetric("foo", "name:bar", "cpu:0")
		m2 := NewMetric("foo", "name:bar", "cpu:1")

		assert.NotEqual(m1, m2)
	})

	t.Run("same name and tags", func(t *testing.T) {
		Clear()
		// tag ordering doesn't matter
		m1 := NewMetric("foo", "name:bar", "cpu:0")
		m2 := NewMetric("foo", "cpu:0", "name:bar")

		assert.Equal(m1, m2)
	})
}

func TestMetricOperations(t *testing.T) {
	assert := assert.New(t)

	t.Run("regular (non-monotonic) metric", func(t *testing.T) {
		Clear()

		m1 := NewMetric("m1")
		m1.Add(int64(5))
		assert.Equal(int64(5), m1.Get())

		m1.Add(int64(5))
		assert.Equal(int64(10), m1.Get())

		v := m1.Swap(int64(0))
		assert.Equal(int64(10), v)
		assert.Equal(int64(0), m1.Get())

		m1.Set(20)
		assert.Equal(int64(20), m1.Get())
	})

	t.Run("monotonic metric", func(t *testing.T) {
		Clear()

		m1 := NewMetric("m1", OptMonotonic)
		m1.Add(int64(5))
		assert.Equal(int64(5), m1.Get())
		assert.Equal(int64(5), m1.Delta())
		assert.Equal(int64(0), m1.Delta())
		assert.Equal(int64(5), m1.Get())

		m1.Add(int64(10))
		assert.Equal(int64(15), m1.Get())
		assert.Equal(int64(10), m1.Delta())
		assert.Equal(int64(0), m1.Delta())
		assert.Equal(int64(15), m1.Get())
	})
}

func TestSplitTagsAndOpts(t *testing.T) {
	assert := assert.New(t)

	t.Run("only tags", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"tag:a", "tag:c", "tag:b"})
		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, tags)
		assert.Len(opts, 0)
	})

	t.Run("only opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions([]string{"_opt3", "_opt2", "_opt1"})
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, opts)
		assert.Len(tags, 0)
	})

	t.Run("tags and opts", func(t *testing.T) {
		tags, opts := splitTagsAndOptions(
			[]string{"_opt3", "tag:a", "_opt2", "tag:b", "_opt1", "tag:c"},
		)

		assert.Equal([]string{"tag:a", "tag:b", "tag:c"}, tags)
		assert.Equal([]string{"_opt1", "_opt2", "_opt3"}, opts)
	})

}
