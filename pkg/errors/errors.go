// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package errors

import "fmt"

type errorReason int

const (
	notFoundError errorReason = iota
	unknownError
)

// agentError is an error intended for consumption by a datadog pkg
type agentError struct {
	error
	message     string
	errorReason errorReason
}

// Error satisfies the error interface
func (e agentError) Error() string {
	return e.message
}

// NewNotFound returns a new error which indicates that the object passed in parameter was not found.
func NewNotFound(notFoundObject string) *agentError {
	return &agentError{
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
	case *agentError:
		return t.errorReason
	}
	return unknownError
}
