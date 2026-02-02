// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock implementation of the depvalidator component
package mock

import (
	depvalidatordef "github.com/DataDog/datadog-agent/comp/logs-library/depvalidator/def"
)

// Mock is a mock implementation of depvalidator.Component
type Mock struct {
	logsEnabled       bool
	validateError     error
	validateCallCount int
	lastValidatedDeps any
}

// Provides is the mock component output
type Provides struct {
	Comp depvalidatordef.Component
}

// NewMock creates a new mock depvalidator component with logs enabled
func NewMock() *Mock {
	return &Mock{
		logsEnabled: true,
	}
}

// NewMockDisabled creates a new mock depvalidator component with logs disabled
func NewMockDisabled() *Mock {
	return &Mock{
		logsEnabled: false,
	}
}

// NewProvides provides a new mock depvalidator component (logs enabled by default)
func NewProvides() Provides {
	return Provides{
		Comp: NewMock(),
	}
}

// NewProvidesDisabled provides a new mock depvalidator component with logs disabled
func NewProvidesDisabled() Provides {
	return Provides{
		Comp: NewMockDisabled(),
	}
}

// LogsEnabled returns the configured logs enabled state
func (m *Mock) LogsEnabled() bool {
	return m.logsEnabled
}

// ValidateDependencies returns the configured error (if any)
func (m *Mock) ValidateDependencies(deps any) error {
	m.validateCallCount++
	m.lastValidatedDeps = deps
	return m.validateError
}

// ValidateIfEnabled combines LogsEnabled and ValidateDependencies
func (m *Mock) ValidateIfEnabled(deps any) error {
	if !m.logsEnabled {
		return depvalidatordef.ErrLogsDisabled
	}
	return m.ValidateDependencies(deps)
}

// SetLogsEnabled sets the logs enabled state for testing
func (m *Mock) SetLogsEnabled(enabled bool) {
	m.logsEnabled = enabled
}

// SetValidateError sets the error to return from ValidateDependencies
func (m *Mock) SetValidateError(err error) {
	m.validateError = err
}

// GetValidateCallCount returns how many times ValidateDependencies was called
func (m *Mock) GetValidateCallCount() int {
	return m.validateCallCount
}

// GetLastValidatedDeps returns the last deps struct passed to ValidateDependencies
func (m *Mock) GetLastValidatedDeps() any {
	return m.lastValidatedDeps
}

// Reset resets the mock state
func (m *Mock) Reset() {
	m.validateCallCount = 0
	m.lastValidatedDeps = nil
	m.validateError = nil
}
