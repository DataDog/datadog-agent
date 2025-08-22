// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"context"
	"math"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func (breaker *CircuitBreaker) Do(ctx context.Context, fn func() error) {
	var attempt int32 = 1

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if attempt > breaker.maxAttempts {
			log.Warnf("%s circuit breaker tripped! waiting %v before continuing...", breaker.name, breaker.waitBeforeRetry)
			sleepWithCtx(ctx, breaker.waitBeforeRetry)
			attempt = 1
			continue
		}

		err := fn()
		if err != nil {
			backoff := time.Duration(float64(breaker.minBackoff) * math.Pow(2, float64(attempt-1)))
			if backoff > breaker.maxBackoff {
				backoff = breaker.maxBackoff
			}

			sleepWithCtx(ctx, backoff)
			attempt++
			continue
		}
		break
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
