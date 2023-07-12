// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package path

import "fmt"

// ErrResolutionNotCritical defines a non critical error
type ErrPathResolutionNotCritical struct {
	Err error
}

// Error implements the error interface
func (e *ErrPathResolutionNotCritical) Error() string {
	return fmt.Errorf("non critical path resolution error: %w", e.Err).Error()
}

// Unwrap implements the error interface
func (e *ErrPathResolutionNotCritical) Unwrap() error {
	return e.Err
}

// ErrPathResolution defines a non critical error
type ErrPathResolution struct {
	Err error
}

// Error implements the error interface
func (e *ErrPathResolution) Error() string {
	return fmt.Errorf("path resolution error: %w", e.Err).Error()
}

// Unwrap implements the error interface
func (e *ErrPathResolution) Unwrap() error {
	return e.Err
}
