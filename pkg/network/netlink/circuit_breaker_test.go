// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package netlink

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func TestCircuitBreakerDefaultState(t *testing.T) {
	breaker := newTestBreaker(100)
	assert.False(t, breaker.IsOpen())
}

func TestCircuitBreakerRemainsClosed(t *testing.T) {
	const maxEventRate = 100
	breaker := newTestBreaker(maxEventRate)

	// The code below simulates a constant rate of events/s during 5 min
	now := time.Now()
	deadline := now.Add(5 * time.Minute)
	for now.Before(deadline) {
		breaker.Tick(maxEventRate)
		breaker.update(now)
		now = now.Add(time.Second)
	}

	//  The circuit breaker should remain closed
	assert.False(t, breaker.IsOpen())
}

func TestCircuitBreakerSupportsBursts(t *testing.T) {
	const maxEventRate = 100
	breaker := newTestBreaker(maxEventRate)

	// Let's assume the circuit-breaker has been running with 80% of the max allowed rate
	now := time.Now()
	breaker.Tick(int(float64(maxEventRate) * 0.8))
	breaker.update(now)
	assert.False(t, breaker.IsOpen())

	// Since we smoothen the event rate using EWMA we shouldn't trip immediately after
	// going above the max rate
	now = now.Add(time.Second)
	deadline := now.Add(3 * time.Second)
	for now.Before(deadline) {
		breaker.Tick(maxEventRate + 5)
		breaker.update(now)
		now = now.Add(time.Second)
	}
	assert.False(t, breaker.IsOpen())

	// However after some time it surely should trip the circuit
	deadline = now.Add(30 * time.Second)
	for now.Before(deadline) {
		breaker.Tick(maxEventRate + 10)
		breaker.update(now)
		now = now.Add(time.Second)
	}
	assert.True(t, breaker.IsOpen())
}

func TestCircuitBreakerReset(t *testing.T) {
	const maxEventRate = 100
	breaker := newTestBreaker(maxEventRate)

	breaker.Tick(maxEventRate * 2)
	breaker.update(time.Now())
	assert.True(t, breaker.IsOpen())

	breaker.Reset()
	assert.False(t, breaker.IsOpen())
}

func TestStartAboveThreshold(t *testing.T) {
	const maxEventRate = 100
	breaker := newTestBreaker(maxEventRate)

	// If our first measurement is above threshold we don't amortize it
	// and trip the circuit.
	breaker.Tick(maxEventRate + 1)
	breaker.update(time.Now())
	assert.True(t, breaker.IsOpen())
}

func TestCircuitBreakerRateCalculation(t *testing.T) {
	const maxEventRate = 60
	breaker := newTestBreaker(maxEventRate)

	// The first use of the breaker doesn't use EWMA, so the event rate should
	// match the first tick
	now := time.Now()
	breaker.Tick(50)
	breaker.update(now)
	assert.Equal(t, int64(50), breaker.Rate())

	// Expected rate (using EWMA weight of 0.2)
	// Rate = 0.2 * 80 (new rate) + 0.8 * 50 (previous rate) = 56
	now = now.Add(time.Second)
	breaker.Tick(80)
	breaker.update(now)
	assert.Equal(t, int64(56), breaker.Rate())

	// Expected rate (using EWMA weight of 0.2)
	// Rate = 0.2 * 80 (new rate) + 0.8 * 56 (previous rate) = ~60
	now = now.Add(time.Second)
	breaker.Tick(80)
	breaker.update(now)
	assert.Equal(t, int64(60), breaker.Rate())

	// Expected rate (using EWMA weight of 0.2)
	// Rate = 0.2 * 80 (new rate) + 0.8 * 60 (previous rate) = ~64
	now = now.Add(time.Second)
	breaker.Tick(80)
	breaker.update(now)
	assert.Equal(t, int64(64), breaker.Rate())

	// At this point the breaker should be open
	assert.True(t, breaker.IsOpen())

	// The calculated rate of the breaker should not change while the breaker is open
	now = now.Add(time.Second)
	breaker.Tick(80)
	breaker.update(now)
	assert.Equal(t, int64(64), breaker.Rate())

	// After resetting the breaker, the rate should be zero again.
	breaker.Reset()
	assert.Equal(t, int64(0), breaker.Rate())
}

func newTestBreaker(maxEventRate int) *CircuitBreaker {
	c := &CircuitBreaker{
		eventCount:      atomic.NewInt64(0),
		eventRate:       atomic.NewInt64(0),
		isOpen:          atomic.NewBool(false),
		lastUpdate:      atomic.NewInt64(0),
		maxEventsPerSec: int64(maxEventRate),
	}
	c.Reset()
	return c
}
