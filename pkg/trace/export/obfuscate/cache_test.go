package obfuscate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMeasuredCacheNoPanic(t *testing.T) {
	// this test mostly ensures that a nil cache creates no panic
	c := measuredCache{Cache: nil}
	ok := c.Set("a", 1, 1)
	assert.False(t, ok)
	_, ok = c.Get("a")
	assert.False(t, ok)
}
