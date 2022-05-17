package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMaxUint64(t *testing.T) {
	assert.Equal(t, uint64(10), MaxUint64(uint64(10), uint64(5)))
	assert.Equal(t, uint64(10), MaxUint64(uint64(5), uint64(10)))
}

func TestMinUint64(t *testing.T) {
	assert.Equal(t, uint64(5), MinUint64(uint64(10), uint64(5)))
	assert.Equal(t, uint64(5), MinUint64(uint64(5), uint64(10)))
}
