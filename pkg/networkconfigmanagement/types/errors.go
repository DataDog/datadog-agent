// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"errors"
	"fmt"
)

// ErrorType indicates what type of error happened
type ErrorType string

const (
	ErrUnknown  ErrorType = ""
	ErrInternal ErrorType = "internal_error"
	ErrDisabled ErrorType = "rollback_disabled"
	// Argument errors
	ErrNoSuchDevice     ErrorType = "unknown_device"     // the DeviceID isn't recognized
	ErrConfigNotPresent ErrorType = "unknown_config"     // the configID isn't in the local store
	ErrWrongDeviceID    ErrorType = "device_id_mismatch" // the config in the local store isn't for this deviceID
	ErrWrongHash        ErrorType = "hash_mismatch"      // the config has doesn't match what's in the store
	// Connection/profile errors
	ErrCannotConnect ErrorType = "cannot_connect" // failed to connect to the device
	ErrNoProfile     ErrorType = "no_profile"     // the device doesn't have a configured profile and no candidate matched
	// ErrProfileNotMatched // the device DOES have an explicitly-configured profile but it doesn't agree with the Verify() method
	ErrPushUnsupported ErrorType = "rollback_not_implemented" // the device's profile doesn't support pushing config
	// Failures during the actual rollback
	ErrCopyFailed       ErrorType = "copy_failed"        // we couldn't copy the configuration to the device
	ErrSetRunningFailed ErrorType = "set_running_failed" // we couldn't set the running config
	ErrSetStartupFailed ErrorType = "set_startup_failed" // we couldn't set the startup config
	// Trailing errors (rollback succeeds but something else goes wrong)
	ErrReportConfigFailed ErrorType = "report_config_failed" // rollback succeeded but something went wrong trying to fetch the configuration afterwards
)

// RollbackError is an error that exposes an ErrorType
type RollbackError interface {
	error
	Type() ErrorType
}

// rollbackWrapper wraps an existing error with an ErrorType
type rollbackWrapper struct {
	errType ErrorType
	wrapped error
}

func (re *rollbackWrapper) Type() ErrorType {
	return re.errType
}

func (re *rollbackWrapper) Error() string {
	return re.wrapped.Error()
}

func (re *rollbackWrapper) Unwrap() error {
	return re.wrapped
}

// WrapError wraps an error in a RollbackError
func WrapError(etype ErrorType, err error) RollbackError {
	return &rollbackWrapper{
		errType: etype,
		wrapped: err,
	}
}

// WrapErrorf is shorthand for WrapError(etype, fmt.Errorf(...))
func WrapErrorf(etype ErrorType, msg string, args ...any) RollbackError {
	return WrapError(etype, fmt.Errorf(msg, args...))
}

var RollbackDisabled = WrapErrorf(ErrDisabled, "rollback is disabled")

// InternalError is a shorthand for wrapping an error with ErrInternal
func InternalError(err error) RollbackError {
	return WrapError(ErrInternal, err)
}

// AsRollbackError is a no-op if error is nil or already a RollbackError,
// otherwise it wraps it with ErrInternal.
func AsRollbackError(err error) RollbackError {
	// no error -> ok
	if err == nil {
		return nil
	}
	// already a rollback error -> nothing to do
	if rberr, ok := errors.AsType[RollbackError](err); ok {
		return rberr
	}
	// not a RollbackError -> wrap with InternalError
	return InternalError(err)

}
