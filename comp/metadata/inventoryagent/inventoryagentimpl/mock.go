// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	iainterface "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
)

// MockModule defines the fx options for the mock component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   inventoryagentimpl.MockModule(),
//	)
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp iainterface.Component
}

type inventoryagentMock struct{}

func newMock() MockProvides {
	ia := &inventoryagentMock{}
	return MockProvides{
		Comp: ia,
	}
}

// Set is an empty function on this mock
func (m *inventoryagentMock) Set(string, interface{}) {}

// GetAsJSON is a mocked function
func (m *inventoryagentMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

// Get is a mocked function
func (m *inventoryagentMock) Get() map[string]interface{} {
	return nil
}

// Refresh is a mocked function
func (m *inventoryagentMock) Refresh() {}
