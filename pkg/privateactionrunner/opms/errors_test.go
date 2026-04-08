// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package opms

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRetryableHTTPError(t *testing.T) {
	cause := errors.New("underlying error")

	t.Run("true for RetryableHTTPError", func(t *testing.T) {
		err := &RetryableHTTPError{StatusCode: 503, cause: cause}
		assert.True(t, IsRetryableHTTPError(err))
	})

	t.Run("true when wrapped with fmt.Errorf %w", func(t *testing.T) {
		inner := &RetryableHTTPError{StatusCode: 429, cause: cause}
		err := fmt.Errorf("enrollment failed: %w", inner)
		assert.True(t, IsRetryableHTTPError(err))
	})

	t.Run("false for plain error", func(t *testing.T) {
		assert.False(t, IsRetryableHTTPError(errors.New("plain error")))
	})

	t.Run("false for nil", func(t *testing.T) {
		assert.False(t, IsRetryableHTTPError(nil))
	})
}

func TestRetryableHTTPError_ErrorAndUnwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &RetryableHTTPError{StatusCode: 503, cause: cause}

	assert.Equal(t, "root cause", err.Error())
	assert.ErrorIs(t, err, cause)
}

func TestIsRetryableStatusCode(t *testing.T) {
	retryable := []int{429, 502, 503, 504}
	for _, code := range retryable {
		assert.True(t, isRetryableStatusCode(code), "expected %d to be retryable", code)
	}

	nonRetryable := []int{200, 400, 401, 403, 404, 413, 500}
	for _, code := range nonRetryable {
		assert.False(t, isRetryableStatusCode(code), "expected %d to be non-retryable", code)
	}
}
