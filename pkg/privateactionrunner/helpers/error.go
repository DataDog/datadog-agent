package helpers

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/errorcode"
)

type PARError struct {
	*errorcode.ActionPlatformError
}

// NewPARError creates a general PAR error with default config.
func NewPARError(code errorcode.ActionPlatformErrorCode, e error) PARError {
	return NewPARErrorWithDisplayError(code, e, e.Error())
}

// NewPARErrorWithDisplayError creates a general PAR error with display error.
func NewPARErrorWithDisplayError(code errorcode.ActionPlatformErrorCode, e error, displayError string) PARError {
	return PARError{
		ActionPlatformError: &errorcode.ActionPlatformError{
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
		return NewPARError(errorcode.ActionPlatformErrorCode_INTERNAL_ERROR, e)
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
		return NewPARErrorWithDisplayError(errorcode.ActionPlatformErrorCode_ACTION_ERROR, e, displayError)
	}
}
