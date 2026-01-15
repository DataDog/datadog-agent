// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosnmplib

// ConnectionError represents a connection error to a device
type ConnectionError struct {
	err error
}

// NewConnectionError creates a new ConnectionError
func NewConnectionError(err error) *ConnectionError {
	return &ConnectionError{
		err: err,
	}
}

// Error returns the error message
func (e *ConnectionError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying error
func (e *ConnectionError) Unwrap() error {
	return e.err
}
