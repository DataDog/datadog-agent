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

type provides struct {
	fx.Out

	Comp     flare.Component
	Endpoint api.AgentEndpointProvider
}

// MockFlare is a mock of the
type MockFlare struct{}

type MockEndpoint struct {
	Comp *MockFlare
}

func (e MockEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
func NewMock() provides {
	m := &MockFlare{}
	e := api.NewAgentEndpointProvider(MockEndpoint{Comp: m}, "/flare", "POST")

	return provides{
		Comp:     m,
		Endpoint: e,
	}
}
