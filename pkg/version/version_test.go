package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	// full fledge
	v, err := New("1.2.3-pre+☢")
	assert.Nil(t, err)
	assert.Equal(t, int64(1), v.Major)
	assert.Equal(t, int64(2), v.Minor)
	assert.Equal(t, int64(3), v.Patch)
	assert.Equal(t, "pre", v.Pre)
	assert.Equal(t, "☢", v.Meta)

	// only pre
	v, err = New("1.2.3-pre-pre.1")
	assert.Nil(t, err)
	assert.Equal(t, "pre-pre.1", v.Pre)

	// only meta
	v, err = New("1.2.3+☢.1+")
	assert.Nil(t, err)
	assert.Equal(t, "☢.1+", v.Meta)

	_, err = New("")
	assert.NotNil(t, err)
	_, err = New("1.2.")
	assert.NotNil(t, err)
	_, err = New("1.2.3.4")
	assert.NotNil(t, err)
	_, err = New("1.2.foo")
	assert.NotNil(t, err)
}

func TestString(t *testing.T) {
	v, _ := New("1.2.3-pre+☢")
	assert.Equal(t, "1.2.3-pre+☢", v.String())
}

func TestGetNumber(t *testing.T) {
	v, _ := New("1.2.3-pre+☢")
	assert.Equal(t, "1.2.3", v.GetNumber())
}
