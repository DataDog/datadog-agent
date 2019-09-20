package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogLimit(t *testing.T) {
	l := NewLogLimit(10, time.Hour)

	now := time.Now()

	for i := 0; i < 10; i++ {
		assert.True(t, l.shouldLogTime(now))
	}

	assert.False(t, l.shouldLogTime(now))
	assert.False(t, l.shouldLogTime(now.Add(time.Minute*20)))

	assert.True(t, l.shouldLogTime(now.Add(time.Minute*61)))
	assert.False(t, l.shouldLogTime(now.Add(time.Minute*61)))
}
