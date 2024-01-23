// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

var statusFormats map[Status]string

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
	panic("not called")
}

// Unwrap implements the Go 1.13 unwrap convention
func (e *Error) Unwrap() error {
	panic("not called")
}

// IsRetryError checks an `error` object to tell if it's a Retry.Error
func IsRetryError(e error) (bool, *Error) {
	panic("not called")
}

// IsErrPermaFail checks whether an `error` is a Retrier permanent fail
func IsErrPermaFail(err error) bool {
	panic("not called")
}

// IsErrWillRetry checks whether an `error` is a Retrier temporary fail
func IsErrWillRetry(err error) bool {
	panic("not called")
}

func init() {
	statusFormats = map[Status]string{
		NeedSetup:     "%s needs to be setup with SetupRetrier: %s",
		FailWillRetry: "temporary failure in %s, will retry later: %s",
		PermaFail:     "permanent failure in %s: %s",
	}
}
