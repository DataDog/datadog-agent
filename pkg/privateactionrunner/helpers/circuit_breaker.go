package helpers

import (
	"context"
	"math"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// CircuitBreaker is a rudimentary circuit breaker that performs retries using exponential backoff.
type CircuitBreaker struct {
	log             log.Component
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
			breaker.log.Warnf("%s circuit breaker tripped! waiting %v before continuing...", breaker.name, breaker.waitBeforeRetry)
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
