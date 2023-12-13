// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package netlink

import (
	"math"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
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
	eventCount *atomic.Int64

	// An exponentially weighted average of the event rate (per second)
	// This is what actually compare against maxEventsPersec
	eventRate *atomic.Int64

	// Represents the status of the cicuit breaker.
	isOpen *atomic.Bool

	// The timestamp in nanoseconds of when we last updated eventRate
	lastUpdate *atomic.Int64

	done chan struct{}
}

// NewCircuitBreaker instantiates a new CircuitBreaker that only allows
// a maxEventsPerSec to pass. The rate of events is calculated using an EWMA.
func NewCircuitBreaker(maxEventsPerSec int64, tickInterval time.Duration) *CircuitBreaker {
	// -1 will virtually disable the circuit breaker
	if maxEventsPerSec == -1 {
		maxEventsPerSec = math.MaxInt64
	}

	c := &CircuitBreaker{
		eventCount:      atomic.NewInt64(0),
		eventRate:       atomic.NewInt64(0),
		isOpen:          atomic.NewBool(false),
		lastUpdate:      atomic.NewInt64(0),
		maxEventsPerSec: maxEventsPerSec,
		done:            make(chan struct{}),
	}
	c.Reset()

	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case t := <-ticker.C:
				c.update(t)
			case <-c.done:
				return
			}
		}
	}()

	return c
}

// IsOpen returns true when the circuit breaker trips and remain
// unchanched until Reset() is called.
func (c *CircuitBreaker) IsOpen() bool {
	return c.isOpen.Load()
}

// Tick represents one or more events passing through the circuit breaker.
func (c *CircuitBreaker) Tick(n int) {
	c.eventCount.Add(int64(n))
}

// Rate returns the current rate of events
func (c *CircuitBreaker) Rate() int64 {
	return c.eventRate.Load()
}

// Reset closes the circuit breaker and its state.
func (c *CircuitBreaker) Reset() {
	c.eventCount.Store(0)
	c.eventRate.Store(0)
	c.lastUpdate.Store(time.Now().UnixNano())
	c.isOpen.Store(false)
}

// Stop stops the circuit breaker.
func (c *CircuitBreaker) Stop() {
	close(c.done)
}

func (c *CircuitBreaker) update(now time.Time) {
	if c.IsOpen() {
		return
	}

	lastUpdate := c.lastUpdate.Load()
	deltaInSec := float64(now.UnixNano()-lastUpdate) / float64(time.Second.Nanoseconds())

	// This is to avoid a divide by 0 panic or a spurious spike due
	// to a reset followed immediately by an update call
	if deltaInSec < 1.0 {
		deltaInSec = 1.0
	}

	// Calculate the event rate (EWMA)
	eventCount := c.eventCount.Swap(0)
	prevEventRate := c.eventRate.Load()
	newEventRate := ewmaWeight*float64(eventCount)/deltaInSec + (1-ewmaWeight)*float64(prevEventRate)

	// If we just started we don't amortize the value.
	// This is to better handle the case where we start above the threshold.
	if prevEventRate == int64(0) {
		newEventRate = float64(eventCount) / deltaInSec
	}

	c.lastUpdate.Store(now.UnixNano())
	c.eventRate.Store(int64(newEventRate))

	// Update circuit breaker status accordingly
	if int64(newEventRate) > c.maxEventsPerSec {
		log.Warnf(
			"exceeded maximum number of netlink messages per second. expected=%d actual=%d",
			c.maxEventsPerSec,
			int(newEventRate),
		)
		c.isOpen.Store(true)
	}
}
