package backoff

import (
	"math"
	"math/rand"
	"time"
)

// Policy contains parameters and logic necessary to implement an exponential backoff
// strategy when handling errors.
type Policy struct {
	// MinBackoffFactor controls the overlap between consecutive retry interval ranges. When
	// set to `2`, there is a guarantee that there will be no overlap. The overlap
	// will asymptotically approach 50% the higher the value is set.
	MinBackoffFactor float64

	// BaseBackoffTime controls the rate of exponential growth. Also, you can calculate the start
	// of the very first retry interval range by evaluating the following expression:
	// baseBackoffTime / minBackoffFactor * 2
	BaseBackoffTime float64

	// MaxBackoffTime is the maximum number of seconds to wait for a retry.
	MaxBackoffTime float64

	// RecoveryInterval controls how many retry interval ranges to step down for an endpoint
	// upon success. Increasing this should only be considered when maxBackoffTime
	// is particularly high or if our intake team is particularly confident.
	RecoveryInterval int

	// MaxErrors derived value is the number of errors it will take to reach the maxBackoffTime.
	MaxErrors int
}

const secondsFloat = float64(time.Second)

func randomBetween(min, max float64) float64 {
	return rand.Float64()*(max-min) + min
}

// NewPolicy constructs new Backoff object with given parameters
func NewPolicy(minBackoffFactor, baseBackoffTime, maxBackoffTime float64, recoveryInterval int, recoveryReset bool) Policy {
	maxErrors := int(math.Floor(math.Log2(maxBackoffTime/baseBackoffTime))) + 1

	if recoveryReset {
		recoveryInterval = maxErrors
	}

	return Policy{
		MinBackoffFactor: minBackoffFactor,
		BaseBackoffTime:  baseBackoffTime,
		MaxBackoffTime:   maxBackoffTime,
		RecoveryInterval: recoveryInterval,
		MaxErrors:        maxErrors,
	}
}

// GetBackoffDuration returns amount of time to sleep after numErrors error
func (b *Policy) GetBackoffDuration(numErrors int) time.Duration {
	var backoffTime float64

	if numErrors > 0 {
		backoffTime = b.BaseBackoffTime * math.Pow(2, float64(numErrors))

		if backoffTime > b.MaxBackoffTime {
			backoffTime = b.MaxBackoffTime
		} else {
			min := backoffTime / b.MinBackoffFactor
			max := math.Min(b.MaxBackoffTime, backoffTime)
			backoffTime = randomBetween(min, max)
		}
	}

	return time.Duration(backoffTime * secondsFloat)

}

// IncError increments the error counter up to MaxErrors
func (b *Policy) IncError(numErrors int) int {
	numErrors++
	if numErrors > b.MaxErrors {
		return b.MaxErrors
	}
	return numErrors
}

// DecError decrements the error counter down to zero at RecoveryInterval rate
func (b *Policy) DecError(numErrors int) int {
	numErrors -= b.RecoveryInterval
	if numErrors < 0 {
		return 0
	}
	return numErrors
}
