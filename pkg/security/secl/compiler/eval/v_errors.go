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

type ErrUnexpectedValueType struct {
	Expected any
	Got      any
}

func (e *ErrUnexpectedValueType) Error() string {
	return fmt.Sprintf("unexpected value type: expected %T, got %T", e.Expected, e.Got)
}

type ErrUnsupportedScope struct {
	VarName string
	Scope   string
}

func (e *ErrUnsupportedScope) Error() string {
	return fmt.Sprintf("variable `%s` has unsupported scope: `%s`", e.VarName, e.Scope)
}

var ErrOperatorNotSupported = errors.New("operation not supported")
