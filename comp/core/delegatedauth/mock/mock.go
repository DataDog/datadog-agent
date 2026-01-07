// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock implementation of the delegatedauth component for testing
package mock

import (
	"testing"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// Mock is a mock implementation of the delegatedauth.Component interface
type Mock struct {
	ConfigureFunc func(delegatedauth.ConfigParams)
}

var _ delegatedauth.Component = (*Mock)(nil)

// Provides is the mock component output
type Provides struct {
	Comp delegatedauth.Component
}

// New creates a new mock delegatedauth component for testing
func New(_ testing.TB) *Mock {
	return &Mock{}
}

// Configure calls the mock function if set, otherwise does nothing
func (m *Mock) Configure(params delegatedauth.ConfigParams) {
	if m.ConfigureFunc != nil {
		m.ConfigureFunc(params)
	}
}
