// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock implementation of the delegatedauth component for testing
package mock

import (
	"context"
	"testing"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// Mock is a mock implementation of the delegatedauth.Component interface
type Mock struct {
	ConfigureFunc     func(delegatedauth.ConfigParams)
	GetAPIKeyFunc     func(ctx context.Context) (*string, error)
	RefreshAPIKeyFunc func(ctx context.Context) error
}

var _ delegatedauth.Component = (*Mock)(nil)

// Provides is the mock component output
type Provides struct {
	Comp delegatedauth.Component
}

// Requires list the required objects to initialize the mock delegatedauth Component
type Requires struct {
	Log interface{} // Accept any log component or nil
}

// New creates a new mock delegatedauth component for testing
func New(_ *testing.T, _ Requires) Provides {
	return Provides{
		Comp: &Mock{},
	}
}

// Configure calls the mock function if set, otherwise does nothing
func (m *Mock) Configure(params delegatedauth.ConfigParams) {
	if m.ConfigureFunc != nil {
		m.ConfigureFunc(params)
	}
}

// GetAPIKey calls the mock function if set, otherwise returns nil
func (m *Mock) GetAPIKey(ctx context.Context) (*string, error) {
	if m.GetAPIKeyFunc != nil {
		return m.GetAPIKeyFunc(ctx)
	}
	return nil, nil
}

// RefreshAPIKey calls the mock function if set, otherwise returns nil
func (m *Mock) RefreshAPIKey(ctx context.Context) error {
	if m.RefreshAPIKeyFunc != nil {
		return m.RefreshAPIKeyFunc(ctx)
	}
	return nil
}
