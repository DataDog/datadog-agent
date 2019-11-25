// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package errors

import "fmt"

type errorReason int

const (
	notFoundError errorReason = iota
	unknownError
)

// AgentError is an error intended for consumption by a datadog pkg; it can also be
// reconstructed by clients from an error response. Public to allow easy type switches.
type AgentError struct {
	error
	message     string
	errorReason errorReason
}

// Error satisfies the error interface
func (e AgentError) Error() string {
	return e.message
}

// NewNotFound returns a new error which indicates that the object passed in parameter was not found.
func NewNotFound(notFoundObject string) *AgentError {
	return &AgentError{
		message:     fmt.Sprintf("%q not found", notFoundObject),
		errorReason: notFoundError,
	}
}

// IsNotFound returns true if the specified error was created by NewNotFound.
func IsNotFound(err error) bool {
	return reasonForError(err) == notFoundError
}

func reasonForError(err error) errorReason {
	switch t := err.(type) {
	case *AgentError:
		return t.errorReason
	}
	return unknownError
}
