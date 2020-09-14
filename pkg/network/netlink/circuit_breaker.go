package netlink

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	tickInterval  = 3 * time.Second
	breakerOpen   = int64(1)
	breakerClosed = int64(0)

	// The lower this number is the more amortized the average is
	// For example, if ewmaWeight is 1, a single burst of events might
	// cause the breaker to trip.
	ewmaWeight = 0.2
)

// CircuitBreaker is meant to enforce a maximum rate of events per second
// Once the event rate goes above the threshold the circuit breaker will trip
// and remain open until Reset() is called.
type CircuitBreaker struct {
	// The maximum rate of events allowed to pass
	maxEventsPerSec int64

	// The number of events elapsed since the last tick
	eventCount int64

	// An exponentially weighted average of the event rate (per second)
	// This is what actually compare against maxEventsPersec
	eventRate int64

	// Represents the status of the cicuit breaker.
	// 1 means open, 0 means closed
	status int64

	// The timestamp in nanoseconds of when we last updated eventRate
	lastUpdate int64
}

// NewCircuitBreaker instantiates a new CircuitBreaker that only allows
// a maxEventsPerSec to pass. The rate of events is calculated using an EWMA.
func NewCircuitBreaker(maxEventsPerSec int64) *CircuitBreaker {
	// -1 will virtually disable the circuit breaker
	if maxEventsPerSec == -1 {
		maxEventsPerSec = math.MaxInt64
	}

	c := &CircuitBreaker{maxEventsPerSec: maxEventsPerSec}
	c.Reset()

	go func() {
		ticker := time.NewTicker(tickInterval)
		for t := range ticker.C {
			c.update(t)
		}
	}()

	return c
}

// IsOpen returns true when the circuit breaker trips and remain
// unchanched until Reset() is called.
func (c *CircuitBreaker) IsOpen() bool {
	return atomic.LoadInt64(&c.status) == breakerOpen
}

// Tick represents one or more events passing through the circuit breaker.
func (c *CircuitBreaker) Tick(n int) {
	atomic.AddInt64(&c.eventCount, int64(n))
}

// Rate returns the current rate of events
func (c *CircuitBreaker) Rate() int64 {
	return atomic.LoadInt64(&c.eventRate)
}

// Reset closes the circuit breaker and its state.
func (c *CircuitBreaker) Reset() {
	atomic.StoreInt64(&c.eventCount, 0)
	atomic.StoreInt64(&c.eventRate, 0)
	atomic.StoreInt64(&c.lastUpdate, time.Now().UnixNano())
	atomic.StoreInt64(&c.status, breakerClosed)
}

func (c *CircuitBreaker) update(now time.Time) {
	if c.IsOpen() {
		return
	}

	lastUpdate := atomic.LoadInt64(&c.lastUpdate)
	deltaInSec := float64(now.UnixNano()-lastUpdate) / float64(time.Second.Nanoseconds())

	// This is to avoid a divide by 0 panic or a spurious spike due
	// to a reset followed immediately by an update call
	if deltaInSec < 1.0 {
		deltaInSec = 1.0
	}

	// Calculate the event rate (EWMA)
	eventCount := atomic.SwapInt64(&c.eventCount, 0)
	prevEventRate := atomic.LoadInt64(&c.eventRate)
	newEventRate := ewmaWeight*float64(eventCount)/deltaInSec + (1-ewmaWeight)*float64(prevEventRate)

	// If we just started we don't amortize the value.
	// This is to better handle the case where we start above the threshold.
	if prevEventRate == int64(0) {
		newEventRate = float64(eventCount) / deltaInSec
	}

	atomic.StoreInt64(&c.lastUpdate, now.UnixNano())
	atomic.StoreInt64(&c.eventRate, int64(newEventRate))

	// Update circuit breaker status accordingly
	if int64(newEventRate) > c.maxEventsPerSec {
		log.Warnf(
			"exceeded maximum number of netlink messages per second. expected=%d actual=%d",
			c.maxEventsPerSec,
			int(newEventRate),
		)
		atomic.StoreInt64(&c.status, breakerOpen)
	}
}
