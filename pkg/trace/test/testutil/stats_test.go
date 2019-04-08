package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomBucket(t *testing.T) {
	for i := 10; i < 100; i += 10 {
		b := RandomBucket(i)
		assert.False(t, b.IsEmpty())
	}
}

func TestTestBucket(t *testing.T) {
	b := TestBucket()
	assert.False(t, b.IsEmpty())
}
