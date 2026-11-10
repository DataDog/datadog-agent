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
	ErrCannotConnect ErrorType = "device_unreachable" // failed to connect to the device
	ErrNoProfile     ErrorType = "no_profile"         // the device doesn't have a configured profile and no candidate matched
	// ErrProfileNotMatched // the device DOES have an explicitly-configured profile but it doesn't agree with the Verify() method
	ErrPushUnsupported ErrorType = "rollback_not_supported" // the device's profile doesn't support pushing config
	// Failures during the actual rollback
	ErrCopyFailed       ErrorType = "copy_failed"        // we couldn't copy the configuration to the device
	ErrSetRunningFailed ErrorType = "set_running_failed" // we couldn't set the running config
	ErrSetStartupFailed ErrorType = "set_startup_failed" // we couldn't set the startup config
	// Trailing errors (rollback succeeds but something else goes wrong)
	ErrReportConfigFailed ErrorType = "report_config_failed" // rollback succeeded but something went wrong trying to fetch the configuration afterwards
	// Errors reporting the NCM check's results
	ErrConfigRetrievalFailed ErrorType = "config_retrieval_failed" // failed to retrieve/process the running or startup config
	ErrMetadataSendFailed    ErrorType = "metadata_send_failed"    // failed to send device metadata to the backend
	ErrPayloadSendFailed     ErrorType = "payload_send_failed"     // failed to send the NCM payload to the backend
)

// TypedError is an error that exposes an ErrorType
type TypedError interface {
	error
	Type() ErrorType
}

// typedWrapper wraps an existing error with an ErrorType
type typedWrapper struct {
	errType ErrorType
	wrapped error
}

func (tw *typedWrapper) Type() ErrorType {
	return tw.errType
}

func (tw *typedWrapper) Error() string {
	return tw.wrapped.Error()
}

func (tw *typedWrapper) Unwrap() error {
	return tw.wrapped
}

// WrapError wraps an error in a TypedError
func WrapError(etype ErrorType, err error) TypedError {
	return &typedWrapper{
		errType: etype,
		wrapped: err,
	}
}

// WrapErrorf is shorthand for WrapError(etype, fmt.Errorf(...))
func WrapErrorf(etype ErrorType, msg string, args ...any) TypedError {
	return WrapError(etype, fmt.Errorf(msg, args...))
}

var RollbackDisabled = WrapErrorf(ErrDisabled, "rollback is disabled")

// InternalError is a shorthand for wrapping an error with ErrInternal
func InternalError(err error) TypedError {
	return WrapError(ErrInternal, err)
}

// AsTypedError is a no-op if error is nil or already a TypedError,
// otherwise it wraps it with ErrInternal.
func AsTypedError(err error) TypedError {
	// no error -> ok
	if err == nil {
		return nil
	}
	// already a typed error -> nothing to do
	if terr, ok := errors.AsType[TypedError](err); ok {
		return terr
	}
	// not a TypedError -> wrap with InternalError
	return InternalError(err)

}
