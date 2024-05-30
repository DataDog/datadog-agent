// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	apidef "github.com/DataDog/datadog-agent/comp/api/api/def"
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

	Comp     iainterface.Component
	Endpoint apidef.AgentEndpointProvider
}

type inventoryagentMock struct{}

// handlerFunc is a simple mocked http.Handler function
func (m *inventoryagentMock) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

func newMock() MockProvides {
	ia := &inventoryagentMock{}
	return MockProvides{
		Comp:     ia,
		Endpoint: apidef.NewAgentEndpointProvider(ia.handlerFunc, "/metadata/inventory-agent", "GET"),
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
