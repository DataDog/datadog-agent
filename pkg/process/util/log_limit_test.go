package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogLimit(t *testing.T) {
	l := NewLogLimit(10, time.Hour)
	defer l.Close()

	for i := 0; i < 10; i++ {
		// this reset will not have any effect because we haven't logged 10 times yet
		l.resetCounter()
		assert.True(t, l.ShouldLog())
	}

	assert.False(t, l.ShouldLog())
	assert.False(t, l.ShouldLog())

	l.resetCounter()
	assert.True(t, l.ShouldLog())
	assert.False(t, l.ShouldLog())
}
