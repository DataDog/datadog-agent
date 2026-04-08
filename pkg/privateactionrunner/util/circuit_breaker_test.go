// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errRetryable = errors.New("retryable error")
var errFatal = errors.New("fatal error")

func alwaysRetry(err error) bool { return true }
func neverRetry(err error) bool  { return false }
func retryOnlyRetryable(err error) bool {
	return errors.Is(err, errRetryable)
}

func newFastBreaker() *CircuitBreaker {
	return NewCircuitBreaker("test", time.Millisecond, 5*time.Millisecond, 10*time.Millisecond, 3)
}

func TestDoWithCondition_SuccessOnFirstCall(t *testing.T) {
	breaker := newFastBreaker()
	calls := 0
	err := breaker.DoWithCondition(context.Background(), func() error {
		calls++
		return nil
	}, alwaysRetry)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestDoWithCondition_RetriesOnRetryableError(t *testing.T) {
	breaker := newFastBreaker()
	calls := 0
	err := breaker.DoWithCondition(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errRetryable
		}
		return nil
	}, retryOnlyRetryable)
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestDoWithCondition_StopsOnNonRetryableError(t *testing.T) {
	breaker := newFastBreaker()
	calls := 0
	err := breaker.DoWithCondition(context.Background(), func() error {
		calls++
		return errFatal
	}, neverRetry)
	assert.ErrorIs(t, err, errFatal)
	assert.Equal(t, 1, calls, "should not retry when shouldRetry returns false")
}

func TestDoWithCondition_StopsOnNonRetryableErrorAfterRetries(t *testing.T) {
	breaker := newFastBreaker()
	calls := 0
	err := breaker.DoWithCondition(context.Background(), func() error {
		calls++
		if calls < 2 {
			return errRetryable
		}
		return errFatal
	}, retryOnlyRetryable)
	assert.ErrorIs(t, err, errFatal)
	assert.Equal(t, 2, calls)
}

func TestDoWithCondition_ReturnsCtxErrWhenCancelled(t *testing.T) {
	breaker := newFastBreaker()
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := breaker.DoWithCondition(ctx, func() error {
		calls++
		cancel() // cancel after first call
		return errRetryable
	}, alwaysRetry)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}

func TestDoWithCondition_ReturnsCtxErrWhenAlreadyCancelled(t *testing.T) {
	breaker := newFastBreaker()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := breaker.DoWithCondition(ctx, func() error {
		calls++
		return nil
	}, alwaysRetry)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, calls, "should not call fn when context is already done")
}

func TestDo_RetriesUntilSuccess(t *testing.T) {
	breaker := newFastBreaker()
	calls := 0
	breaker.Do(context.Background(), func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	assert.Equal(t, 3, calls)
}

func TestDo_StopsOnContextCancel(t *testing.T) {
	breaker := newFastBreaker()
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	breaker.Do(ctx, func() error {
		calls++
		cancel()
		return errors.New("always fails")
	})

	assert.Equal(t, 1, calls)
}
