// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package inventoryhostimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   inventoryhost.MockModule(),
//	)
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp inventoryhost.Component
}

type inventoryhostMock struct{}

func (m *inventoryhostMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (m *inventoryhostMock) Refresh() {}

func newMock() MockProvides {
	ih := &inventoryhostMock{}
	return MockProvides{
		Comp: ih,
	}
}
