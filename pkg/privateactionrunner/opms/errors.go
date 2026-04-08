// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package opms

import "errors"

// RetryableHTTPError indicates the enrollment HTTP request failed with a
// transient error that is safe to retry (e.g. 429, 502, 503, 504, or a network
// error). StatusCode is 0 for network-level failures.
type RetryableHTTPError struct {
	StatusCode int
	cause      error
}

func (e *RetryableHTTPError) Error() string { return e.cause.Error() }
func (e *RetryableHTTPError) Unwrap() error { return e.cause }

// IsRetryableHTTPError reports whether err is (or wraps) a
// RetryableHTTPError.
func IsRetryableHTTPError(err error) bool {
	var retryable *RetryableHTTPError
	return errors.As(err, &retryable)
}

// isRetryableStatusCode returns true for HTTP status codes that represent
// transient server-side or rate-limiting conditions worth retrying.
// 500 Internal Server Error is intentionally excluded as it typically
// indicates a server-side bug that retrying will not resolve.
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case 429, 502, 503, 504:
		return true
	default:
		return false
	}
}
