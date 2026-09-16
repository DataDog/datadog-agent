// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

// RetryableError represents an error that can occur when sending a payload.
type RetryableError struct {
	err error
}

// NewRetryableError returns a new destination error.
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{
		err: err,
	}
}

// RetryableError returns the message of the error.
func (e *RetryableError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error so that errors.Is and errors.As can traverse
// RetryableError. Without this, callers can match neither the inner sentinel
// (e.g. a package-level errServer) nor any wrapped *url.Error / *net.OpError —
// which made error-classification helpers fall back to message-string heuristics
// and miscategorise wrapped HTTP-status errors as "other".
func (e *RetryableError) Unwrap() error {
	return e.err
}
