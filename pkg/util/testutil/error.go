// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package testutil provides various helper functions for tests
package testutil

// ErrorString is a simple error implementation that can be used in tests
type ErrorString struct {
	s string
}

// Error returns the error string
func (e *ErrorString) Error() string {
	return e.s
}

// Is returns true if the error string is equal to the other error
func (e *ErrorString) Is(other error) bool {
	return e.Error() == other.Error()
}

// NewErrorString creates a new error with the given string
func NewErrorString(s string) error {
	return &ErrorString{s}
}
