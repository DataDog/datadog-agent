// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides mock implementation of the dogstatsd server component
package mock

import (
	server "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Mock implements mock-specific methods.
type Mock interface {
	server.Component
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

// NewMock creates a new mock dogstatsd server component
func NewMock() server.Component {
	return newMock().Comp
}
