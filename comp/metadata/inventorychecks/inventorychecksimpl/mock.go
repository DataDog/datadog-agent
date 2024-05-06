// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorychecksimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api"
	icinterface "github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp     icinterface.Component
	Endpoint api.AgentEndpointProvider
}

// InventorychecksMock mocks methods for the inventorychecks components for testing
type InventorychecksMock struct {
	metadata map[string]map[string]interface{}
}

// NewMock returns a new InventorychecksMock.
// TODO: (components) - Once the checks are components we can make this method private
func NewMock() MockProvides {
	ic := &InventorychecksMock{
		metadata: map[string]map[string]interface{}{},
	}
	return MockProvides{
		Comp:     ic,
		Endpoint: api.NewAgentEndpointProvider(ic.handlerFunc, "/metadata/inventory-checks", "GET"),
	}
}

// handlerFunc is a simple mocked http.Handler function
func (m *InventorychecksMock) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// Set sets a metadata value for a specific instancID
func (m *InventorychecksMock) Set(instanceID string, key string, value interface{}) {
	if _, found := m.metadata[instanceID]; !found {
		m.metadata[instanceID] = map[string]interface{}{}
	}
	m.metadata[instanceID][key] = value
}

// Refresh is a empty method for the inventorychecks mock
func (m *InventorychecksMock) Refresh() {}

// GetAsJSON returns an hardcoded empty JSON dict
func (m *InventorychecksMock) GetAsJSON() ([]byte, error) { return []byte("{}"), nil }

// GetInstanceMetadata returns all the metadata set for an instanceID using the Set method
func (m *InventorychecksMock) GetInstanceMetadata(instanceID string) map[string]interface{} {
	if metadata, found := m.metadata[instanceID]; found {
		return metadata
	}
	return nil
}

// MockModule defines the fx options for the mock component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   inventorychecks.MockModule(),
//	)
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock))
}
