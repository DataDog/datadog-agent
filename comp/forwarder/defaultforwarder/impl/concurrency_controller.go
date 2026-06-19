// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const (
	// initialLimit is the concurrency the controller starts (and restarts) at.
	initialLimit = 1
	// evalInterval is how often the controller evaluates whether to grow the
	// limit. It is sized to the payload arrival cadence so that a single batch
	// arriving between ticks does not look like sustained backlog.
	evalInterval = 15 * time.Second
	// saturationThreshold is the fraction of an evaluation window the semaphore
	// must spend fully saturated before the limit grows.
	saturationThreshold = 0.8
	// increaseStep is the additive growth applied per evaluation window.
	increaseStep = 1
	// minDecreaseInterval debounces decreases: a burst of backoff signals (e.g.
	// every transaction in a batch getting a 429) collapses into a single
	// halving, while sustained throttling keeps stepping the limit down.
	minDecreaseInterval = 1 * time.Second
	// increaseCooldown suppresses growth for this long after a decrease, so the
	// limit does not immediately climb back into the throttle it just yielded to.
	increaseCooldown = evalInterval
)

// concurrencyController scales a resizableSemaphore's limit at runtime using an
// AIMD policy: additive increase when the semaphore is sustainedly saturated
// (we cannot keep up with the arrival rate), and an immediate multiplicative
// decrease when a backoff signal (HTTP 408/429 or a timeout) is observed.
//
// All limit mutations happen in the run goroutine, so the current limit needs
// no additional locking.
type concurrencyController struct {
	log      log.Component
	sem      *resizableSemaphore
	domain   string
	maxLimit int64
	backoff  <-chan struct{}
	now      func() time.Time

	stop chan struct{}
	done chan struct{}

	limit        int64
	lastEval     time.Time
	lastDecrease time.Time
}

func newConcurrencyController(
	log log.Component,
	sem *resizableSemaphore,
	domain string,
	maxLimit int64,
	backoff <-chan struct{},
) *concurrencyController {
	return &concurrencyController{
		log:      log,
		sem:      sem,
		domain:   domain,
		maxLimit: maxLimit,
		backoff:  backoff,
		now:      time.Now,
	}
}

// start resets the limit to initialLimit and launches the control loop. It is
// safe to call again after stop.
func (c *concurrencyController) start() {
	c.stop = make(chan struct{})
	c.done = make(chan struct{})
	c.limit = initialLimit
	c.lastEval = c.now()
	c.lastDecrease = time.Time{}
	c.sem.SetLimit(c.limit)
	tlmConcurrencyLimit.Set(float64(c.limit), c.domain)
	// Drain any backoff signal left over from before start so it does not
	// immediately halve the freshly reset limit.
	c.drainBackoff()

	go c.run()
}

// stopController stops the control loop and waits for it to exit.
func (c *concurrencyController) stopController() {
	if c.stop == nil {
		return
	}
	close(c.stop)
	<-c.done
	c.stop = nil
}

func (c *concurrencyController) run() {
	defer close(c.done)

	ticker := time.NewTicker(evalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-c.backoff:
			c.onBackoff()
		case <-ticker.C:
			c.onEval()
		}
	}
}

// onBackoff reacts to a 408/429/timeout by halving the limit immediately,
// debounced so a burst collapses into one step.
func (c *concurrencyController) onBackoff() {
	now := c.now()
	if !c.lastDecrease.IsZero() && now.Sub(c.lastDecrease) < minDecreaseInterval {
		return
	}
	c.lastDecrease = now
	c.setLimit(max(1, c.limit/2))
}

// onEval grows the limit when the semaphore was saturated for most of the
// window, unless we recently backed off.
func (c *concurrencyController) onEval() {
	now := c.now()
	window := now.Sub(c.lastEval)
	c.lastEval = now

	saturated := c.sem.takeSaturatedDuration()
	if window <= 0 {
		return
	}
	if !c.lastDecrease.IsZero() && now.Sub(c.lastDecrease) < increaseCooldown {
		return
	}
	if float64(saturated)/float64(window) >= saturationThreshold {
		c.setLimit(min(c.maxLimit, c.limit+increaseStep))
	}
}

func (c *concurrencyController) setLimit(limit int64) {
	if limit == c.limit {
		return
	}
	c.limit = limit
	c.sem.SetLimit(limit)
	tlmConcurrencyLimit.Set(float64(limit), c.domain)
	c.log.Debugf("Forwarder concurrency limit for domain %q set to %d", c.domain, limit)
}

func (c *concurrencyController) drainBackoff() {
	for {
		select {
		case <-c.backoff:
		default:
			return
		}
	}
}
