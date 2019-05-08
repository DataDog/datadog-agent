package backoff

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var errBogus = fmt.Errorf("bogus error")

func TestExponentialDelay(t *testing.T) {
	assert := assert.New(t)

	conf := ExponentialConfig{
		// Use nanoseconds to reduce universe from which randoms are chosen. Seconds should be the same, just scaled.
		MaxDuration: 120 * time.Nanosecond,
		GrowthBase:  2,
		Base:        time.Nanosecond,
	}

	// Use fixed random to prevent flakiness in case the CI has very bad luck
	delayProvider := exponentialDelayProviderCustomRandom(conf, rand.New(rand.NewSource(1234)))

	prevMax := int64(0)

	// Try successive calls to delayProvider with increasing numRetries (from 0 to 19).
	for i := 0; i < 20; i++ {
		expectedMax := int64(math.Pow(2, float64(i)))

		if expectedMax > int64(conf.MaxDuration) {
			expectedMax = int64(conf.MaxDuration)
		}

		// For each value of numRetries, get min and max value we saw over 500 calls
		min, max := minMaxForSample(delayProvider, 500, i)

		assert.True(max <= expectedMax, "Max should be lower or equal to expected max. Max: %d, expected: %d", max,
			expectedMax)
		assert.True(max >= prevMax, "Max should grow because this is exp. backoff. Current: %d, prev: %d",
			max, prevMax)
		assert.True(min <= max/2, "Minimum should be 'far' from max since this should be jittery. Min: %d, max: %d",
			min, max)

		prevMax = max
	}
}

func TestExponentialOverflow(t *testing.T) {
	assert := assert.New(t)

	delayProvider := DefaultExponentialDelayProvider()

	assert.NotPanics(func() {
		min, max := minMaxForSample(delayProvider, 300, 1024)

		assert.True(min >= 0, "Min should be greater or equal to 0")
		assert.True(max <= int64(DefaultExponentialConfig().MaxDuration), "Min should be greater or equal to 0")
	})
}

func minMaxForSample(delayProvider DelayProvider, n int, numTries int) (min, max int64) {
	max = 0
	min = math.MaxInt64

	for i := 0; i < n; i++ {
		delay := int64(delayProvider(numTries, nil))

		if delay > max {
			max = delay
		}

		if delay < min {
			min = delay
		}
	}

	return
}
