// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package errors contains errors used by the updater.
package errors

import (
	"errors"
)

type updaterErrorCode uint64

const (
	errUnknown updaterErrorCode = iota // This error code is purposefully not exported
	ErrInstallFailed
	ErrDownloadFailed
	ErrInvalidHash
	ErrInvalidState
	ErrPackageNotFound
	ErrUpdateExperimentFailed
)

type UpdaterError struct {
	err  error
	code updaterErrorCode
}

// Error returns the error message.
func (e UpdaterError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e UpdaterError) Unwrap() error {
	return e.err
}

func (e UpdaterError) Is(target error) bool {
	_, ok := target.(*UpdaterError)
	return ok
}

func (e UpdaterError) Code() updaterErrorCode {
	return e.code
}

// Wrap wraps the given error with an updater error.
// If the given error is already an updater error, it is not wrapped and
// left as it is. Only the deepest UpdaterError remains.
func Wrap(errCode updaterErrorCode, err error) error {
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
	e, ok := err.(*UpdaterError)
	if !ok {
		return &UpdaterError{
			err:  err,
			code: errUnknown,
		}
	}
	return e
}
