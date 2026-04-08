// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"context"
	"math"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
)

// CircuitBreaker is a rudimentary circuit breaker that performs retries using exponential backoff.
type CircuitBreaker struct {
	name            string
	minBackoff      time.Duration
	maxBackoff      time.Duration
	waitBeforeRetry time.Duration
	maxAttempts     int32
}

func NewCircuitBreaker(name string, minBackoff, maxBackoff, waitBeforeRetry time.Duration, maxAttempts int32) *CircuitBreaker {
	return &CircuitBreaker{
		name:            name,
		minBackoff:      minBackoff,
		maxBackoff:      maxBackoff,
		waitBeforeRetry: waitBeforeRetry,
		maxAttempts:     maxAttempts,
	}
}

// Do runs fn with exponential backoff, retrying on any error until fn succeeds
// or ctx is cancelled.
func (breaker *CircuitBreaker) Do(ctx context.Context, fn func() error) {
	alwaysRetry := func(error) bool { return true }
	_ = breaker.DoWithCondition(ctx, fn, alwaysRetry)
}

// DoWithCondition runs fn with exponential backoff, retrying only when
// shouldRetry(err) returns true. It returns nil on success, the first
// non-retryable error immediately, or ctx.Err() if the context is cancelled
// while waiting to retry. Retries indefinitely on retryable errors.
func (breaker *CircuitBreaker) DoWithCondition(ctx context.Context, fn func() error, shouldRetry func(error) bool) error {
	var attempt int32 = 1

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if attempt > breaker.maxAttempts {
			log.FromContext(ctx).Warnf("%s circuit breaker tripped! waiting %v before continuing...", breaker.name, breaker.waitBeforeRetry)
			sleepWithCtx(ctx, breaker.waitBeforeRetry)
			attempt = 1
			continue
		}

		err := fn()
		if err == nil {
			return nil
		}

		if !shouldRetry(err) {
			return err
		}

		backoff := time.Duration(float64(breaker.minBackoff) * math.Pow(2, float64(attempt-1)))
		if backoff > breaker.maxBackoff {
			backoff = breaker.maxBackoff
		}

		sleepWithCtx(ctx, backoff)
		attempt++
	}
}

func sleepWithCtx(ctx context.Context, duration time.Duration) {
	t := time.NewTimer(duration)
	select {
	case <-ctx.Done():
		t.Stop()
	case <-t.C:
	}
}
