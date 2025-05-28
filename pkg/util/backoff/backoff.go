// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package backoff provides backoff mechanisms
package backoff

import (
	"math"
	"math/rand"
	"time"
)

// ExpBackoffPolicy contains parameters and logic necessary to implement an exponential backoff
// strategy when handling errors.
type ExpBackoffPolicy struct {
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

// NewExpBackoffPolicy constructs new Backoff object with given parameters
func NewExpBackoffPolicy(minBackoffFactor, baseBackoffTime, maxBackoffTime float64, recoveryInterval int, recoveryReset bool) Policy {
	maxErrors := int(math.Floor(math.Log2(maxBackoffTime/baseBackoffTime))) + 1

	if recoveryReset {
		recoveryInterval = maxErrors
	}

	return &ExpBackoffPolicy{
		MinBackoffFactor: minBackoffFactor,
		BaseBackoffTime:  baseBackoffTime,
		MaxBackoffTime:   maxBackoffTime,
		RecoveryInterval: recoveryInterval,
		MaxErrors:        maxErrors,
	}
}

// GetBackoffDuration returns amount of time to sleep after numErrors error
func (e *ExpBackoffPolicy) GetBackoffDuration(numErrors int) time.Duration {
	var backoffTime float64

	if numErrors > 0 {
		backoffTime = e.BaseBackoffTime * math.Pow(2, float64(numErrors))

		if backoffTime > e.MaxBackoffTime {
			backoffTime = e.MaxBackoffTime
		} else {
			min := backoffTime / e.MinBackoffFactor
			max := math.Min(e.MaxBackoffTime, backoffTime)
			backoffTime = randomBetween(min, max)
		}
	}

	return time.Duration(backoffTime * secondsFloat)

}

// IncError increments the error counter up to MaxErrors
func (e *ExpBackoffPolicy) IncError(numErrors int) int {
	numErrors++
	if numErrors > e.MaxErrors {
		return e.MaxErrors
	}
	return numErrors
}

// DecError decrements the error counter down to zero at RecoveryInterval rate
func (e *ExpBackoffPolicy) DecError(numErrors int) int {
	numErrors -= e.RecoveryInterval
	if numErrors < 0 {
		return 0
	}
	return numErrors
}
