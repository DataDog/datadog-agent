// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package backoff

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(10)
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
		assert.True(t, max >= between)
	}
}

func TestEmpty(t *testing.T) {
	b := ExpBackoffPolicy{}
	assert.Equal(t, 0, b.IncError(0))
	assert.Equal(t, 0, b.DecError(0))
	assert.Equal(t, time.Duration(0), b.GetBackoffDuration(0))
}

func TestBackoff(t *testing.T) {
	b := NewExpBackoffPolicy(1, 1, 9, 2, false)

	assert.Equal(t, 1, b.IncError(0))
	assert.Equal(t, 2, b.IncError(1))
	assert.Equal(t, 3, b.IncError(2))
	assert.Equal(t, 4, b.IncError(3))
	assert.Equal(t, 4, b.IncError(4))

	assert.Equal(t, 0, b.DecError(0))
	assert.Equal(t, 0, b.DecError(1))
	assert.Equal(t, 0, b.DecError(2))
	assert.Equal(t, 1, b.DecError(3))
	assert.Equal(t, 2, b.DecError(4))

	assert.Equal(t, 0*time.Second, b.GetBackoffDuration(0))
	assert.Equal(t, 2*time.Second, b.GetBackoffDuration(1))
	assert.Equal(t, 4*time.Second, b.GetBackoffDuration(2))
	assert.Equal(t, 8*time.Second, b.GetBackoffDuration(3))
	assert.Equal(t, 9*time.Second, b.GetBackoffDuration(4))
}
