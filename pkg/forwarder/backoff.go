// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

	// This controls the rate of exponential growth. Also, you can calculate the start
	// of the very first retry interval range by evaluating the following expression:
	// baseBackoffTime / minBackoffFactor * 2
	baseBackoffTime = 2

	// This is the maximum number of seconds to wait for a retry.
	maxBackoffTime = 64

	secondsFloat = float64(time.Second)
)

// This is the number of attempts it will take to reach the maxBackoffTime. Our
// blockedEndpoints circuit breaker uses this value as the maximum number of errors.
var maxAttempts = int(math.Ceil(math.Log2(float64(maxBackoffTime) / float64(baseBackoffTime))))

func randomBetween(min, max float64) float64 {
	return rand.Float64()*(max-min) + min
}

// GetBackoffDuration returns an appropriate amount of time to wait for the next network
// error retry given the current number of attempts. Unlike `github.com/cenkalti/backoff`,
// this implementation is thread-safe.
func GetBackoffDuration(numAttempts int) time.Duration {
	var backoffTime float64

	if numAttempts > 0 {
		backoffTime = baseBackoffTime * math.Pow(2, float64(numAttempts))

		if backoffTime > maxBackoffTime {
			backoffTime = maxBackoffTime
		} else {
			min := backoffTime / minBackoffFactor
			max := math.Min(maxBackoffTime, backoffTime)
			backoffTime = randomBetween(min, max)
		}
	}

	return time.Duration(backoffTime * secondsFloat)
}
