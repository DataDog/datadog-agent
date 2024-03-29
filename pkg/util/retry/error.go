// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import "fmt"

// Error is a custom error type that is returned by the Retrier
// you can get its Status with IsRetryError()
type Error struct {
	LogicError    error
	LastTryError  error
	RessourceName string
	RetryStatus   Status
}

// Error implements the `error` interface
func (e *Error) Error() string {
	var format string
	switch e.RetryStatus {
	case NeedSetup:
		format = "%s needs to be setup with SetupRetrier: %s"
	case FailWillRetry:
		format = "temporary failure in %s, will retry later: %s"
	case PermaFail:
		format = "permanent failure in %s: %s"
	default:
		format = "error in %s: %s"
	}
	return fmt.Sprintf(format, e.RessourceName, e.LogicError)
}

// Unwrap implements the Go 1.13 unwrap convention
func (e *Error) Unwrap() error {
	return e.LastTryError
}

// IsRetryError checks an `error` object to tell if it's a Retry.Error
func IsRetryError(e error) (bool, *Error) {
	err, ok := e.(*Error)
	if ok {
		return true, err
	}
	return false, nil
}

// IsErrPermaFail checks whether an `error` is a Retrier permanent fail
func IsErrPermaFail(err error) bool {
	ok, e := IsRetryError(err)
	if !ok {
		return false
	}
	return (e.RetryStatus == PermaFail)
}

// IsErrWillRetry checks whether an `error` is a Retrier temporary fail
func IsErrWillRetry(err error) bool {
	ok, e := IsRetryError(err)
	if !ok {
		return false
	}
	return (e.RetryStatus == FailWillRetry)
}
