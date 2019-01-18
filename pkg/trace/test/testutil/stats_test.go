package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomStatsBucket(t *testing.T) {
	for i := 10; i < 100; i += 10 {
		b := RandomStatsBucket(i)
		assert.False(t, b.IsEmpty())
	}
}

func TestTestStatsBucket(t *testing.T) {
	b := TestStatsBucket()
	assert.False(t, b.IsEmpty())
}
