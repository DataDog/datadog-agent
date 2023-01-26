// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build serverless
// +build serverless

package appsec

import (
	"time"

	"go.uber.org/atomic"
)

// Limiter is used to abstract the rate limiter implementation to only expose the needed function for rate limiting.
// This is for example useful for testing, allowing us to use a modified rate limiter tuned for testing through the same
// interface.
type Limiter interface {
	Allow() bool
}

// TokenTicker is a thread-safe and lock-free rate limiter based on a token bucket.
// The idea is to have a goroutine that will update  the bucket with fresh tokens at regular intervals using a time.Ticker.
// The advantage of using a goroutine here is  that the implementation becomes easily thread-safe using a few
// atomic operations with little overhead overall. TokenTicker.Start() *should* be called before the first call to
// TokenTicker.Allow() and TokenTicker.Stop() *must* be called once done using. Note that calling TokenTicker.Allow()
// before TokenTicker.Start() is valid, but it means the bucket won't be refilling until the call to TokenTicker.Start() is made
type TokenTicker struct {
	tokens    atomic.Int64
	maxTokens int64
	ticker    *time.Ticker
	stopChan  chan struct{}
}

// NewTokenTicker is a utility function that allocates a token ticker, initializes necessary fields and returns it
func NewTokenTicker(tokens, maxTokens int64) *TokenTicker {
	return &TokenTicker{
		tokens:    *atomic.NewInt64(tokens),
		maxTokens: maxTokens,
	}
}

// updateBucket performs a select loop to update the token amount in the bucket.
// Used in a goroutine by the rate limiter.
func (t *TokenTicker) updateBucket(ticksChan <-chan time.Time, startTime time.Time, syncChan chan struct{}) {
	nsPerToken := time.Second.Nanoseconds() / t.maxTokens
	elapsedNs := int64(0)
	prevStamp := startTime

	for {
		select {
		case <-t.stopChan:
			if syncChan != nil {
				close(syncChan)
			}
			return
		case stamp := <-ticksChan:
			// Compute the time in nanoseconds that passed between the previous timestamp and this one
			// This will be used to know how many tokens can be added into the bucket depending on the limiter rate
			elapsedNs += stamp.Sub(prevStamp).Nanoseconds()
			if elapsedNs > t.maxTokens*nsPerToken {
				elapsedNs = t.maxTokens * nsPerToken
			}
			prevStamp = stamp
			// Update the number of tokens in the bucket if enough nanoseconds have passed
			if elapsedNs >= nsPerToken {
				// Atomic spin lock to make sure we don't race for `t.tokens`
				for {
					tokens := t.tokens.Load()
					if tokens == t.maxTokens {
						break // Bucket is already full, nothing to do
					}
					inc := elapsedNs / nsPerToken
					// Make sure not to add more tokens than we are allowed to into the bucket
					if tokens+inc > t.maxTokens {
						inc -= (tokens + inc) % t.maxTokens
					}
					if t.tokens.CompareAndSwap(tokens, tokens+inc) {
						// Keep track of remaining elapsed ns that were not taken into account for this computation,
						// so that increment computation remains precise over time
						elapsedNs = elapsedNs % nsPerToken
						break
					}
				}
			}
			// Sync channel used to signify that the goroutine is done updating the bucket. Used for tests to guarantee
			// that the goroutine ticked at least once.
			if syncChan != nil {
				syncChan <- struct{}{}
			}
		}
	}
}

// Start starts the ticker and launches the goroutine responsible for updating the token bucket.
// The ticker is set to tick at a fixed rate of 500us.
func (t *TokenTicker) Start() {
	timeNow := time.Now()
	t.ticker = time.NewTicker(500 * time.Microsecond)
	t.start(t.ticker.C, timeNow, false)
}

// start is used for internal testing. Controlling the ticker means being able to test per-tick
// rather than per-duration, which is more reliable if the app is under a lot of stress.
// sync is used to decide whether the limiter should create a channel for synchronization with the testing app after a
// bucket update. The limiter is in charge of closing the channel in this case.
func (t *TokenTicker) start(ticksChan <-chan time.Time, startTime time.Time, sync bool) <-chan struct{} {
	t.stopChan = make(chan struct{})
	var syncChan chan struct{}

	if sync {
		syncChan = make(chan struct{})
	}
	go t.updateBucket(ticksChan, startTime, syncChan)
	return syncChan
}

// Stop shuts down the rate limiter, taking care stopping the ticker and closing all channels
func (t *TokenTicker) Stop() {
	// Stop the ticker only if it has been instantiated (not the case when testing by calling start() directly)
	if t.ticker != nil {
		t.ticker.Stop()
	}
	// Close the stop channel only if it has been created. This covers the case where Stop() is called without any prior
	// call to Start()
	if t.stopChan != nil {
		close(t.stopChan)
	}
}

// Allow checks and returns whether a token can be retrieved from the bucket and consumed.
// Thread-safe.
func (t *TokenTicker) Allow() bool {
	for {
		tokens := t.tokens.Load()
		if tokens == 0 {
			return false
		} else if t.tokens.CompareAndSwap(tokens, tokens-1) {
			return true
		}
	}
}
