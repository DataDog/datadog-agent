// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides mock implementation of the process apiserver component
package mock

import (
	apiserver "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Mock implements mock-specific methods.
type Mock interface {
	apiserver.Component
}

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
	)
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return Module()
}

// NewMock creates a new mock process apiserver component
func NewMock() apiserver.Component {
	return &mockAPIServer{}
}

type mockAPIServer struct{}

// Mock implementation - apiserver component has no exposed methods, so this is just an empty struct
