// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the delegatedauth mock component
package fx

import (
	"go.uber.org/fx"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockProvides struct {
	fx.Out

	Comp delegatedauth.Component
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

// newMock returns a new mock for the delegated auth component
func newMock() mockProvides {
	return mockProvides{
		Comp: &mockDelegatedAuth{},
	}
}

type mockDelegatedAuth struct{}

var _ delegatedauth.Component = (*mockDelegatedAuth)(nil)

// Configure is a no-op for the mock
func (m *mockDelegatedAuth) Configure(_ delegatedauth.ConfigParams) {}
