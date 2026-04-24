// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the flare component.
package mock

import (
	"net/http"
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	flaredef "github.com/DataDog/datadog-agent/comp/core/flare/def"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the mock component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
	)
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp     flaredef.Component
	Endpoint api.AgentEndpointProvider
}

// MockFlare is a mock of the flare component.
type MockFlare struct{}

// handlerFunc is a simple mocked http.Handler function
func (fc *MockFlare) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// Create mocks the flare create function
func (fc *MockFlare) Create(_ flaretypes.ProfileData, _ time.Duration, _ error, _ []byte) (string, error) {
	return "", nil
}

// Send mocks the flare send function
func (fc *MockFlare) Send(_ string, _ string, _ string, _ flaretypes.FlareSource) (string, error) {
	return "", nil
}

// CreateWithArgs mocks the flare create with args function
func (fc *MockFlare) CreateWithArgs(_ flaretypes.FlareArgs, _ time.Duration, _ error, _ []byte) (string, error) {
	return "", nil
}

// NewMock returns a new flare provider
func NewMock() MockProvides {
	m := &MockFlare{}

	return MockProvides{
		Comp:     m,
		Endpoint: api.NewAgentEndpointProvider(m.handlerFunc, "/flare", "POST"),
	}
}
