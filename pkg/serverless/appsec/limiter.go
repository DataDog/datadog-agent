// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

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
	panic("not called")
}

// updateBucket performs a select loop to update the token amount in the bucket.
// Used in a goroutine by the rate limiter.
func (t *TokenTicker) updateBucket(ticksChan <-chan time.Time, startTime time.Time, syncChan chan struct{}) {
	panic("not called")
}

// Start starts the ticker and launches the goroutine responsible for updating the token bucket.
// The ticker is set to tick at a fixed rate of 500us.
func (t *TokenTicker) Start() {
	panic("not called")
}

// start is used for internal testing. Controlling the ticker means being able to test per-tick
// rather than per-duration, which is more reliable if the app is under a lot of stress.
// sync is used to decide whether the limiter should create a channel for synchronization with the testing app after a
// bucket update. The limiter is in charge of closing the channel in this case.
func (t *TokenTicker) start(ticksChan <-chan time.Time, startTime time.Time, sync bool) <-chan struct{} {
	panic("not called")
}

// Stop shuts down the rate limiter, taking care stopping the ticker and closing all channels
func (t *TokenTicker) Stop() {
	panic("not called")
}

// Allow checks and returns whether a token can be retrieved from the bucket and consumed.
// Thread-safe.
func (t *TokenTicker) Allow() bool {
	panic("not called")
}
