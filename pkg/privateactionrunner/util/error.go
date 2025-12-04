// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"errors"
	"fmt"

	aperrorpb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

type PARError struct {
	*aperrorpb.ActionPlatformError
}

// NewPARError creates a general PAR error with default config.
func NewPARError(code aperrorpb.ActionPlatformErrorCode, e error) PARError {
	return NewPARErrorWithDisplayError(code, e, e.Error())
}

// NewPARErrorWithDisplayError creates a general PAR error with display error.
func NewPARErrorWithDisplayError(code aperrorpb.ActionPlatformErrorCode, e error, displayError string) PARError {
	return PARError{
		ActionPlatformError: &aperrorpb.ActionPlatformError{
			ErrorCode:         code,
			Retryable:         false,
			Message:           e.Error(),
			ExternalMessage:   displayError,
			DependencyMessage: "",
		},
	}
}

func (pe PARError) Error() string {
	return fmt.Sprintf("Error:%s, ExternalMessage: %s", pe.Message, pe.ExternalMessage)
}

// DefaultPARError generates the default PAR error with a default error code, and default internal error message.
func DefaultPARError(e error) PARError {
	var pe PARError
	if errors.As(e, &pe) {
		return pe
	} else {
		return NewPARError(aperrorpb.ActionPlatformErrorCode_INTERNAL_ERROR, e)
	}
}

// DefaultActionError generates the default PAR action error with a default action error code.
func DefaultActionError(e error) PARError {
	return DefaultActionErrorWithDisplayError(e, e.Error())
}

// DefaultActionErrorWithDisplayError generates the default PAR action error with a default action error code
// and display error.
func DefaultActionErrorWithDisplayError(e error, displayError string) PARError {
	var pe PARError
	if errors.As(e, &pe) {
		return pe
	} else {
		return NewPARErrorWithDisplayError(aperrorpb.ActionPlatformErrorCode_ACTION_ERROR, e, displayError)
	}
}
