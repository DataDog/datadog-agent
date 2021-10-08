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
