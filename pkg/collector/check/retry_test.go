// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryableErrorMessage(t *testing.T) {
	inner := errors.New("connection refused")
	err := RetryableError{Err: inner}
	assert.Equal(t, "connection refused", err.Error())
}

func TestRetrySucceedsImmediately(t *testing.T) {
	calls := 0
	err := Retry(time.Second, 3, func() error {
		calls++
		return nil
	}, "test")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRetryNonRetryableError(t *testing.T) {
	calls := 0
	err := Retry(time.Second, 3, func() error {
		calls++
		return errors.New("fatal error")
	}, "test")
	assert.Error(t, err)
	assert.Equal(t, "fatal error", err.Error())
	assert.Equal(t, 1, calls) // no retry for non-retryable errors
}

func TestRetryBailsOutAfterMaxRetries(t *testing.T) {
	calls := 0
	err := Retry(time.Second, 3, func() error {
		calls++
		return RetryableError{Err: errors.New("transient")}
	}, "mycheck")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bail out from mycheck")
	assert.Contains(t, err.Error(), "max retries reached")
	// initial call + (retries-1) retries, then bail on the retries-th attempt
	assert.Equal(t, 3, calls)
}

func TestRetrySucceedsAfterRetries(t *testing.T) {
	calls := 0
	err := Retry(time.Second, 5, func() error {
		calls++
		if calls < 3 {
			return RetryableError{Err: errors.New("transient")}
		}
		return nil
	}, "test")
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}
