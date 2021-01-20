package flush

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAtLeast(t *testing.T) {
	assert := assert.New(t)

	// flush should happen at least every 2 second
	s := &AtLeast{N: 2 * time.Second}
	s.lastFlush = time.Now().Add(-time.Second * 10)

	assert.True(s.ShouldFlush(Starting, time.Now()), "it should flush because last flush was 10 seconds ago")

	s.lastFlush = time.Now().Add(-time.Second * 60)
	assert.True(s.ShouldFlush(Starting, time.Now()), "it should flush because last flush was 1 minute ago")

	s.lastFlush = time.Now().Add(-time.Second)
	assert.False(s.ShouldFlush(Starting, time.Now()), "it should not flush because last flush was less than 2 second ago")
}
