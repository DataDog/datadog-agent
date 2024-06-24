// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package inventoryhostimpl

import (
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
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

	Comp     inventoryhost.Component
	Endpoint api.AgentEndpointProvider
}

type inventoryhostMock struct{}

// handlerFunc is a simple mocked http.Handler function
func (m *inventoryhostMock) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

func (m *inventoryhostMock) GetAsJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (m *inventoryhostMock) Refresh() {}

func newMock() MockProvides {
	ih := &inventoryhostMock{}
	return MockProvides{
		Comp:     ih,
		Endpoint: api.NewAgentEndpointProvider(ih.handlerFunc, "/metadata/inventory-host", "GET"),
	}
}
