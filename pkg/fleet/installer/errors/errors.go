// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package errors contains errors used by the installer.
package errors

import (
	"encoding/json"
	"errors"
)

// InstallerErrorCode is an error code used by the installer.
type InstallerErrorCode uint64

const (
	errUnknown InstallerErrorCode = 0 // This error code is purposefully not exported
	// ErrDownloadFailed is the code for a download failure.
	ErrDownloadFailed InstallerErrorCode = 1
	// ErrNotEnoughDiskSpace is the code for not enough disk space.
	ErrNotEnoughDiskSpace InstallerErrorCode = 2
	// ErrPackageNotFound is the code for a package not found.
	ErrPackageNotFound InstallerErrorCode = 3
	// ErrFilesystemIssue is the code for a filesystem issue (e.g. permission issue).
	ErrFilesystemIssue InstallerErrorCode = 4
	// ErrConfigNotFound is the code for a config not found.
	ErrConfigNotFound InstallerErrorCode = 5
)

// InstallerError is an error type used by the installer.
type InstallerError struct {
	err  error
	code InstallerErrorCode
}

type installerErrorJSON struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// Error returns the error message.
func (e InstallerError) Error() string {
	return e.err.Error()
}

// Is implements the Is method of the errors.Is interface.
func (e InstallerError) Is(target error) bool {
	_, ok := target.(*InstallerError)
	return ok
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

// GetCode returns the installer error code of the given error.
func GetCode(err error) InstallerErrorCode {
	code := errUnknown
	e := &InstallerError{}
	if ok := errors.As(err, &e); ok {
		code = e.code
	}
	return code
}

// ToJSON returns the error as a JSON string.
func ToJSON(err error) string {
	tmp := installerErrorJSON{
		Error: err.Error(),
		Code:  int(GetCode(err)),
	}
	jsonErr, err := json.Marshal(tmp)
	if err != nil {
		return err.Error()
	}
	return string(jsonErr)
}

// FromJSON returns an InstallerError from a JSON string.
func FromJSON(errStr string) *InstallerError {
	var jsonError installerErrorJSON
	err := json.Unmarshal([]byte(errStr), &jsonError)
	if err != nil {
		return &InstallerError{
			err:  errors.New(errStr),
			code: errUnknown,
		}
	}
	return &InstallerError{
		err:  errors.New(jsonError.Error),
		code: InstallerErrorCode(jsonError.Code),
	}
}
