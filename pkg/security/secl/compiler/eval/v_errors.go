// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
)

// ErrUnexpectedValueType represents an invalid variable type assignment
type ErrUnexpectedValueType struct {
	Expected any
	Got      any
}

// Error returns the error message of the error
func (e *ErrUnexpectedValueType) Error() string {
	return fmt.Sprintf("unexpected value type: expected %T, got %T", e.Expected, e.Got)
}

// ErrUnsupportedScope represents an unsupported scope error
type ErrUnsupportedScope struct {
	VarName string
	Scope   string
}

// Error returns the error message of the error
func (e *ErrUnsupportedScope) Error() string {
	return fmt.Sprintf("variable `%s` has unsupported scope: `%s`", e.VarName, e.Scope)
}

// ErrOperatorNotSupported represents an invalid variable assignment
var ErrOperatorNotSupported = errors.New("operation not supported")

// ErrScopeFailure wraps an error coming from a variable scoper
type ErrScopeFailure struct {
	VarName    string
	ScoperType InternalScoperType
	ScoperErr  error
}

// Error returns the error message of the error
func (e *ErrScopeFailure) Error() string {
	return fmt.Sprintf("failed to get scope `%s` of variable `%s`: %s", e.ScoperType.String(), e.VarName, e.ScoperErr)
}

// Unwrap unwraps the error
func (e *ErrScopeFailure) Unwrap() error {
	return e.ScoperErr
}
