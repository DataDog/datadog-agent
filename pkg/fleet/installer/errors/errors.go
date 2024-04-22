// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package errors contains errors used by the installer.
package errors

import (
	"errors"
)

// InstallerErrorCode is an error code used by the installer.
type InstallerErrorCode uint64

const (
	errUnknown InstallerErrorCode = iota // This error code is purposefully not exported
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

// InstallerError is an error type used by the installer.
type InstallerError struct {
	err  error
	code InstallerErrorCode
}

// Error returns the error message.
func (e InstallerError) Error() string {
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e InstallerError) Unwrap() error {
	return e.err
}

// Is implements the Is method of the errors.Is interface.
func (e InstallerError) Is(target error) bool {
	_, ok := target.(*InstallerError)
	return ok
}

// Code returns the error code of the installer error.
func (e InstallerError) Code() InstallerErrorCode {
	return e.code
}

// Wrap wraps the given error with an installer error.
// If the given error is already an installer error, it is not wrapped and
// left as it is. Only the deepest InstallerError remains.
func Wrap(errCode InstallerErrorCode, err error) error {
	if errors.Is(err, &InstallerError{}) {
		return err
	}
	return &InstallerError{
		err:  err,
		code: errCode,
	}
}

// From returns a new InstallerError from the given error.
func From(err error) *InstallerError {
	if err == nil {
		return nil
	}

	e, ok := err.(*InstallerError)
	if !ok {
		return &InstallerError{
			err:  err,
			code: errUnknown,
		}
	}
	return e
}
