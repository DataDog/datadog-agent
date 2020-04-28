package netlink

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetGen(t *testing.T) {
	now := time.Unix(1000, 0)
	generationLength := time.Nanosecond * 100

	genNow := getCurrentGeneration(generationLength, now.UnixNano())

	genPlusOne := getNthGeneration(generationLength, now.UnixNano(), 1)
	assert.Equal(t, genNow+1, genPlusOne)

	genNowPlusOneNs := getCurrentGeneration(generationLength, now.UnixNano()+1)
	assert.Equal(t, genNow, genNowPlusOneNs)

	genPlus150Ns := getCurrentGeneration(generationLength, now.UnixNano()+150)
	assert.Equal(t, genNow+1, genPlus150Ns)

	// test generations wrap around
	genNowPlus255 := getCurrentGeneration(generationLength, now.Add(255*generationLength).UnixNano())
	assert.Equal(t, genNow, genNowPlus255)
}
