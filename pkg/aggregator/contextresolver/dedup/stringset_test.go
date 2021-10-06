package dedup

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
)

func TestStringSet(t *testing.T) {
	ss := NewStringSet()

	a0 := "a"
	b0 := "b"
	a1 := "a"
	b1 := "b"

	pa0 := ss.Get(a0)
	assert.Equal(t, a0, *pa0)  // Values are the same

	pa1 := ss.Get(a1)
	assert.Equal(t, pa0, pa1)  // pointers are now the same
	assert.Equal(t, a1, *pa1)  // Values are the same
	assert.Equal(t, 1, ss.Size())  // only one string

	pb0 := ss.Get(b0)
	assert.Equal(t, 2, ss.Size())
	assert.Equal(t, b0, *pb0)

	ss.Dec(pb0)
	assert.Equal(t, 1, ss.Size())

	pb1 := ss.Get(b1)
	assert.Equal(t, 2, ss.Size())
	assert.Equal(t, b1, *pb1)

	ss.Clear()
	assert.Equal(t, 0, ss.Size())
}
