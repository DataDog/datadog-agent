// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package errors contains errors used by the updater.
package errors

import (
	"errors"
)

// UpdaterErrorCode is an error code used by the updater.
type UpdaterErrorCode uint64

const (
	errUnknown UpdaterErrorCode = iota // This error code is purposefully not exported
	// ErrInstallFailed is the code for an install failure.
	ErrInstallFailed
	// ErrDownloadFailed is the code for a download failure.
	ErrDownloadFailed
	// ErrInvalidHash is the code for an invalid hash.
	ErrInvalidHash
	// ErrInvalidState is the code for an invalid state.
	ErrInvalidState
	// ErrPackageNotFound is the code for a package not found.
	ErrPackageNotFound
	// ErrUpdateExperimentFailed is the code for an update experiment failure.
	ErrUpdateExperimentFailed
)

// UpdaterError is an error type used by the updater.
type UpdaterError struct {
	err  error
	code UpdaterErrorCode
}

// Error returns the error message.
func (e UpdaterError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e UpdaterError) Unwrap() error {
	return e.err
}

// Is implements the Is method of the errors.Is interface.
func (e UpdaterError) Is(target error) bool {
	_, ok := target.(*UpdaterError)
	return ok
}

// Code returns the error code of the updater error.
func (e UpdaterError) Code() UpdaterErrorCode {
	return e.code
}

// Wrap wraps the given error with an updater error.
// If the given error is already an updater error, it is not wrapped and
// left as it is. Only the deepest UpdaterError remains.
func Wrap(errCode UpdaterErrorCode, err error) error {
	if errors.Is(err, &UpdaterError{}) {
		return err
	}
	return &UpdaterError{
		err:  err,
		code: errCode,
	}
}

// From returns a new UpdaterError from the given error.
func From(err error) *UpdaterError {
	if err == nil {
		return nil
	}

	e, ok := err.(*UpdaterError)
	if !ok {
		return &UpdaterError{
			err:  err,
			code: errUnknown,
		}
	}
	return e
}
