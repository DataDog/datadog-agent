// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package common implements utilities shared across the e2e tests
package common

import "fmt"

// InternalError is an error type used to wrap internal errors
type InternalError struct {
	Err error
}

// Error returns a printable InternalError
func (i InternalError) Error() string {
	return fmt.Sprintf("E2E INTERNAL ERROR: %v", i.Err)
}

// Is returns true if the target error is an InternalError
func (i InternalError) Is(target error) bool {
	_, ok := target.(InternalError)
	return ok
}
