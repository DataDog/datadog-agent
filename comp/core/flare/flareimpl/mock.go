// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package flareimpl implements the component flare
package flareimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
	)
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp     flare.Component
	Endpoint api.AgentEndpointProvider
}

// MockFlare is a mock of the
type MockFlare struct{}

// MockEndpoint wraps the flare mock with the http.Handler interface
type MockEndpoint struct {
	Comp *MockFlare
}

// ServeHTTP is a simple mocked http.Handler function
func (e MockEndpoint) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// Create mocks the flare create function
func (fc *MockFlare) Create(_ flare.ProfileData, _ error) (string, error) {
	return "a string", nil
}

// Send mocks the flare send function
func (fc *MockFlare) Send(_ string, _ string, _ string, _ helpers.FlareSource) (string, error) {
	return "a string", nil
}

// NewMock returns a new flare provider
func NewMock() MockProvides {
	m := &MockFlare{}
	e := api.NewAgentEndpointProvider(MockEndpoint{Comp: m}, "/flare", "POST")

	return MockProvides{
		Comp:     m,
		Endpoint: e,
	}
}
