// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosnmplib

type ConnectionError struct {
	err error
}

func NewConnectionError(err error) *ConnectionError {
	return &ConnectionError{
		err: err,
	}
}

func (e *ConnectionError) Error() string {
	return e.err.Error()
}

func (e *ConnectionError) Unwrap() error {
	return e.err
}
