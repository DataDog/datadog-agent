package pack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackString(t *testing.T) {
	p := NewStringPacker()

	assert.Equal(t, "a", p.Pack("a"))
	assert.Equal(t, "b", p.Pack("b"))
	assert.Equal(t, "^0", p.Pack("a"))
	assert.Equal(t, "c", p.Pack("c"))
	assert.Equal(t, "^1", p.Pack("b"))
	assert.Equal(t, "^0", p.Pack("a"))
}

func TestUnpackString(t *testing.T) {
	u := NewStringUnPacker()

	assert.Equal(t, "a", u.UnPack("a"))
	assert.Equal(t, "b", u.UnPack("b"))
	assert.Equal(t, "a", u.UnPack("^0"))
	assert.Equal(t, "c", u.UnPack("c"))
	assert.Equal(t, "b", u.UnPack("^1"))
	assert.Equal(t, "a", u.UnPack("^0"))
}
