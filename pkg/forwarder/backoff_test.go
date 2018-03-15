// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestMinBackoffFactorValid(t *testing.T) {
	assert.True(t, minBackoffFactor >= 2)
}

func TestRandomBetween(t *testing.T) {
	getRandomMinMax := func() (float64, float64) {
		a := float64(rand.Intn(10))
		b := float64(rand.Intn(10))
		min := math.Min(a, b)
		max := math.Max(a, b)
		return min, max
	}

	for i := 1; i < 100; i++ {
		min, max := getRandomMinMax()
		between := randomBetween(min, max)

		assert.True(t, min <= between)
		assert.True(t, max > between)
	}
}

func TestGetBackoffDuration(t *testing.T) {
	previousBackoffDuration := time.Duration(0) * time.Second
	backoffIncrease := 0
	backoffDecrease := 0

	for i := 1; ; i++ {
		backoffTime := baseBackoffTime * math.Pow(2, float64(i))

		if backoffTime > maxBackoffTime {
			backoffTime = maxBackoffTime
		} else {
			min := backoffTime / minBackoffFactor
			max := math.Min(maxBackoffTime, backoffTime)
			backoffTime = randomBetween(min, max)
		}

		expectedBackoffDuration := time.Duration(backoffTime * secondsFloat)
		assert.Equal(t, expectedBackoffDuration, GetBackoffDuration(i))

		if i > 1000 {
			assert.Truef(t, i < 1000, "Too many iterations")
		} else if expectedBackoffDuration == previousBackoffDuration {
			break
		} else {
			if expectedBackoffDuration > previousBackoffDuration {
				backoffIncrease++
			} else {
				backoffDecrease++
			}
			previousBackoffDuration = expectedBackoffDuration
		}
	}

	assert.True(t, backoffIncrease >= backoffDecrease)
}
