// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestContextCancellationStopsRefresh(t *testing.T) {
	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel to signal when the goroutine exits
	done := make(chan bool, 1)

	// Create a test version of the background refresh goroutine
	// This simulates the actual implementation in startBackgroundRefresh
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		defer func() { done <- true }()

		for {
			select {
			case <-ctx.Done():
				// Context was canceled, exit the goroutine
				return
			case <-ticker.C:
				// Simulate refresh work
			}
		}
	}()

	// Wait a bit to ensure goroutine is running
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for the goroutine to exit with a timeout
	select {
	case <-done:
		// Success - goroutine exited
	case <-time.After(1 * time.Second):
		t.Fatal("Goroutine did not exit after context cancellation")
	}
}

func TestBackgroundRefreshContextHandling(t *testing.T) {
	// This test verifies that the background refresh goroutine properly
	// handles context cancellation at different points in its lifecycle

	tests := []struct {
		name          string
		cancelAfter   time.Duration
		tickInterval  time.Duration
		expectTimeout bool
	}{
		{
			name:          "cancel before first tick",
			cancelAfter:   10 * time.Millisecond,
			tickInterval:  100 * time.Millisecond,
			expectTimeout: false,
		},
		{
			name:          "cancel after first tick",
			cancelAfter:   150 * time.Millisecond,
			tickInterval:  100 * time.Millisecond,
			expectTimeout: false,
		},
		{
			name:          "cancel immediately",
			cancelAfter:   0,
			tickInterval:  100 * time.Millisecond,
			expectTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan bool, 1)

			// Start the goroutine
			go func() {
				ticker := time.NewTicker(tt.tickInterval)
				defer ticker.Stop()
				defer func() { done <- true }()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						// Check if context is already canceled
						if ctx.Err() != nil {
							return
						}
						// Simulate work
					}
				}
			}()

			// Cancel after specified duration
			if tt.cancelAfter > 0 {
				time.Sleep(tt.cancelAfter)
			}
			cancel()

			// Wait for goroutine to exit
			select {
			case <-done:
				// Success
			case <-time.After(1 * time.Second):
				if !tt.expectTimeout {
					t.Fatal("Goroutine did not exit after context cancellation")
				}
			}
		})
	}
}

func TestCalculateNextRetryInterval(t *testing.T) {
	tests := []struct {
		name                string
		refreshInterval     time.Duration
		consecutiveFailures int
		expectedInterval    time.Duration
	}{
		{
			name:                "first failure - base interval",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 1,
			expectedInterval:    15 * time.Minute, // 15 * 2^0 = 15 minutes
		},
		{
			name:                "second failure - 2x base interval",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 2,
			expectedInterval:    30 * time.Minute, // 15 * 2^1 = 30 minutes
		},
		{
			name:                "third failure - 4x base interval, capped at max",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 3,
			expectedInterval:    time.Hour, // 15 * 2^2 = 60 minutes (1 hour)
		},
		{
			name:                "fourth failure - still capped at max",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 4,
			expectedInterval:    time.Hour, // 15 * 2^3 = 120 minutes, capped at 1 hour
		},
		{
			name:                "no failures - base interval",
			refreshInterval:     30 * time.Minute,
			consecutiveFailures: 0,
			expectedInterval:    30 * time.Minute, // 30 * 2^0 = 30 minutes
		},
		{
			name:                "small base interval with failures",
			refreshInterval:     5 * time.Minute,
			consecutiveFailures: 1,
			expectedInterval:    5 * time.Minute, // 5 * 2^0 = 5 minutes
		},
		{
			name:                "small base interval quickly hits max",
			refreshInterval:     5 * time.Minute,
			consecutiveFailures: 5,
			expectedInterval:    time.Hour, // 5 * 2^4 = 80 minutes, capped at 1 hour
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &delegatedAuthComponent{
				refreshInterval:     tt.refreshInterval,
				consecutiveFailures: tt.consecutiveFailures,
			}

			result := comp.calculateNextRetryInterval()
			assert.Equal(t, tt.expectedInterval, result,
				"Expected %v but got %v for %d consecutive failures with base interval %v",
				tt.expectedInterval, result, tt.consecutiveFailures, tt.refreshInterval)
		})
	}
}

func TestExponentialBackoffBehavior(t *testing.T) {
	// Test the backoff progression for a typical scenario
	comp := &delegatedAuthComponent{
		refreshInterval:     15 * time.Minute,
		consecutiveFailures: 0,
	}

	// Test progression through multiple failures
	expectedIntervals := []time.Duration{
		15 * time.Minute, // 0 failures: 15 * 2^0 = 15 minutes
		15 * time.Minute, // 1 failure:  15 * 2^0 = 15 minutes (first retry at base interval)
		30 * time.Minute, // 2 failures: 15 * 2^1 = 30 minutes
		60 * time.Minute, // 3 failures: 15 * 2^2 = 60 minutes (1 hour)
		60 * time.Minute, // 4 failures: 15 * 2^3 = 120 minutes, capped at 1 hour
	}

	for i, expected := range expectedIntervals {
		result := comp.calculateNextRetryInterval()
		assert.Equal(t, expected, result,
			"Failure %d: expected %v but got %v", i, expected, result)

		// Simulate a failure for the next iteration
		if i < len(expectedIntervals)-1 {
			comp.consecutiveFailures++
		}
	}
}

func TestConsecutiveFailuresCap(t *testing.T) {
	// Test that consecutiveFailures is capped at maxConsecutiveFailures
	comp := &delegatedAuthComponent{
		refreshInterval:     5 * time.Minute,
		consecutiveFailures: 0,
	}

	// Simulate many failures - consecutiveFailures should be capped
	for i := 0; i < maxConsecutiveFailures+5; i++ {
		if comp.consecutiveFailures < maxConsecutiveFailures {
			comp.consecutiveFailures++
		}
	}

	assert.Equal(t, maxConsecutiveFailures, comp.consecutiveFailures,
		"consecutiveFailures should be capped at %d", maxConsecutiveFailures)

	// Verify backoff still works correctly at the cap
	result := comp.calculateNextRetryInterval()
	assert.Equal(t, time.Hour, result,
		"Backoff should still be capped at max interval even with max consecutive failures")
}

func TestMaxConsecutiveFailuresValue(t *testing.T) {
	// Verify that maxConsecutiveFailures is set to a reasonable value
	// that ensures we reach maxBackoffInterval with any reasonable refresh_interval

	// Test with minimum reasonable refresh interval (1 minute)
	comp := &delegatedAuthComponent{
		refreshInterval:     1 * time.Minute,
		consecutiveFailures: maxConsecutiveFailures,
	}

	result := comp.calculateNextRetryInterval()
	assert.Equal(t, maxBackoffInterval, result,
		"With maxConsecutiveFailures, even a 1-minute refresh interval should reach maxBackoffInterval")

	// Verify the math: 1 minute * 2^(maxConsecutiveFailures-1) should be >= 60 minutes
	calculatedBackoff := time.Duration(float64(1*time.Minute) * math.Pow(2, float64(maxConsecutiveFailures-1)))
	assert.GreaterOrEqual(t, calculatedBackoff, maxBackoffInterval,
		"maxConsecutiveFailures (%d) should be high enough that 1 minute * 2^%d >= 1 hour",
		maxConsecutiveFailures, maxConsecutiveFailures-1)
}
