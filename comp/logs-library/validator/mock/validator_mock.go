// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the validator component
package mock

import (
	"github.com/DataDog/datadog-agent/comp/logs-library/validator/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Provides contains the mock validator component.
type Provides struct {
	def.Component
}

// MockModule always returns success (no error) for all validation calls
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(NewMockValidator),
	)
}

// MockValidator is a mock implementation of the validator component.
type MockValidator struct{}

// NewMockValidator creates a new mock validator instance.
func NewMockValidator() Provides {
	return Provides{
		Component: &MockValidator{},
	}
}

// ValidateDependencies always returns nil (success) for the mock validator.
func (v *MockValidator) ValidateDependencies(_ ...def.Option) error {
	return nil
}
