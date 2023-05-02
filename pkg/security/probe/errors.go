// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

// ErrNoProcessContext defines an error for event without process context
type ErrProcessContext struct {
	Err error
}

// Error implements the error interface
func (e *ErrProcessContext) Error() string {
	return e.Err.Error()
}

// Unwrap implements the error interface
func (e *ErrProcessContext) Unwrap() error {
	return e.Err
}
