package netlink

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreakerDefaultState(t *testing.T) {
	breaker := NewCircuitBreaker(100)
	assert.False(t, breaker.IsOpen())
}

func TestCircuitBreakerRemainsClosed(t *testing.T) {
	const maxEventRate = 100
	breaker := NewCircuitBreaker(maxEventRate)

	// The code below simulates a constant rate of events/s during 5 min
	now := time.Now()
	deadline := now.Add(5 * time.Minute)
	for tt := now; tt.Before(deadline); tt = tt.Add(time.Second) {
		breaker.Tick(maxEventRate)
		breaker.update(tt)
	}

	//  The circuit breaker should remain closed
	assert.False(t, breaker.IsOpen())
}

func TestCircuitBreakerSupportsBursts(t *testing.T) {
	const maxEventRate = 100
	breaker := NewCircuitBreaker(maxEventRate)

	// Since we smoothen the event rate using EWMA we shouldn't trip immediately after
	// going above the max rate
	now := time.Now()
	deadline := now.Add(3 * time.Second)
	for tt := now; tt.Before(deadline); tt = tt.Add(time.Second) {
		breaker.Tick(maxEventRate + 10)
		breaker.update(tt)
	}
	assert.False(t, breaker.IsOpen())

	// However after some time it surely should trip the circuit
	deadline = now.Add(30 * time.Second)
	for tt := now; tt.Before(deadline); tt = tt.Add(time.Second) {
		breaker.Tick(maxEventRate + 10)
		breaker.update(tt)
	}
	assert.True(t, breaker.IsOpen())
}

func TestCircuitBreakerReset(t *testing.T) {
	const maxEventRate = 100
	breaker := NewCircuitBreaker(maxEventRate)

	now := time.Now()
	deadline := now.Add(time.Minute)
	for tt := now; tt.Before(deadline); tt = tt.Add(time.Second) {
		breaker.Tick(maxEventRate * 2)
		breaker.update(tt)
	}

	assert.True(t, breaker.IsOpen())
	breaker.Reset()
	assert.False(t, breaker.IsOpen())
}
