// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastTestOpts uses sub-millisecond intervals so tests don't spend real time waiting.
func fastTestOpts(maxElapsed time.Duration) RetryHTTPOptions {
	return RetryHTTPOptions{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     2 * time.Millisecond,
		MaxElapsedTime:  maxElapsed,
	}
}

func TestRetryHTTPRequest_SuccessFirstTry(t *testing.T) {
	var calls int32
	result, err := RetryHTTPRequest(context.Background(), func() (string, int, error) {
		atomic.AddInt32(&calls, 1)
		return "ok", 200, nil
	}, fastTestOpts(0))

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.EqualValues(t, 1, atomic.LoadInt32(&calls))
}

func TestRetryHTTPRequest_RetriesOn5xxThenSucceeds(t *testing.T) {
	var calls int32
	result, err := RetryHTTPRequest(context.Background(), func() (string, int, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return "", 503, errors.New("service unavailable")
		}
		return "ok", 200, nil
	}, fastTestOpts(0))

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.EqualValues(t, 3, atomic.LoadInt32(&calls))
}

func TestRetryHTTPRequest_RetriesOnTransportErrorThenSucceeds(t *testing.T) {
	var calls int32
	result, err := RetryHTTPRequest(context.Background(), func() (string, int, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			return "", 0, errors.New("dial tcp: connection refused")
		}
		return "ok", 200, nil
	}, fastTestOpts(0))

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.EqualValues(t, 2, atomic.LoadInt32(&calls))
}

func TestRetryHTTPRequest_NoRetryOn4xx(t *testing.T) {
	var calls int32
	originalErr := errors.New("bad credentials")
	_, err := RetryHTTPRequest(context.Background(), func() (string, int, error) {
		atomic.AddInt32(&calls, 1)
		return "", 401, originalErr
	}, fastTestOpts(0))

	require.Error(t, err)
	assert.ErrorIs(t, err, originalErr)
	assert.EqualValues(t, 1, atomic.LoadInt32(&calls), "4xx should not retry")
}

func TestRetryHTTPRequest_StopsAtMaxElapsedTime(t *testing.T) {
	var calls int32
	originalErr := errors.New("server error")
	start := time.Now()
	_, err := RetryHTTPRequest(context.Background(), func() (string, int, error) {
		atomic.AddInt32(&calls, 1)
		return "", 500, originalErr
	}, fastTestOpts(50*time.Millisecond))

	elapsed := time.Since(start)
	require.Error(t, err)
	assert.ErrorIs(t, err, originalErr)
	assert.Greater(t, atomic.LoadInt32(&calls), int32(1), "should retry at least once")
	assert.Less(t, elapsed, 1*time.Second, "should stop within MaxElapsedTime budget")
}

func TestRetryHTTPRequest_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	go func() {
		// Cancel after a few attempts.
		for atomic.LoadInt32(&calls) < 2 {
			time.Sleep(1 * time.Millisecond)
		}
		cancel()
	}()

	_, err := RetryHTTPRequest(ctx, func() (string, int, error) {
		atomic.AddInt32(&calls, 1)
		return "", 500, errors.New("transient")
	}, fastTestOpts(0)) // unbounded, only ctx exits

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
