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

	baseBackoff    = 2
	maxBackoffTime = 64
	secondsFloat   = float64(time.Second)
)

func randomBetween(min, max float64) float64 {
	return rand.Float64() * (max - min) + min
}

// GetBackoffDuration returns an appropriate amount of time to wait for the next network
// error retry given the current number of attempts. Unlike `github.com/cenkalti/backoff`,
// this implementation is thread-safe.
func GetBackoffDuration(numAttempts int) time.Duration {
	backoffTime := baseBackoff * math.Pow(2, float64(numAttempts))
	min := backoffTime / minBackoffFactor
	max := math.Min(maxBackoffTime, backoffTime)
	return time.Duration(randomBetween(min, max) * secondsFloat)
}
