package forwarder

import (
	"math"
	"math/rand"
	"time"
)

const (
	// The `minBackoffFactor` controls the overlap between consecutive interval ranges.
	// When set to `2`, there is a guarantee that there will be no overlap. The overlap
	// will asymptotically approach 50% the higher the value is set.
	minBackoffFactor = 2

	baseBackoff    = 2
	maxBackoffTime = 64
	secondsFloat   = float64(time.Second)
)

func randomBetween(min, max float64) float64 {
	return rand.Float64() * (max - min) + min
}

func getBackoffDuration(numAttempts int) time.Duration {
	backoffTime := baseBackoff * math.Pow(2, float64(numAttempts))
	min := backoffTime / minBackoffFactor
	max := math.Min(maxBackoffTime, backoffTime)
	return time.Duration(randomBetween(min, max) * secondsFloat)
}
