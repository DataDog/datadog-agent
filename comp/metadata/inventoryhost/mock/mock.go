// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the inventoryhost component
package mock

import (
	"go.uber.org/fx"

	inventoryhost "github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements mock-specific methods for the inventoryhost component.
type Mock interface {
	inventoryhost.Component
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp inventoryhost.Component
}

type inventoryhostMock struct{}

func (m *inventoryhostMock) Refresh() {}

func newMock() MockProvides {
	ih := &inventoryhostMock{}
	return MockProvides{
		Comp: ih,
	}
}

// Module defines the fx options for the mock component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   mock.Module(),
//	)
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}
