// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package hostimpl

import (
	"context"
	"net/http"

	"go.uber.org/fx"

	apihelper "github.com/DataDog/datadog-agent/comp/api/api/helpers"
	hostinterface "github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mocked component
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock))
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp          hostinterface.Component
	Endpoint      apihelper.AgentEndpointProvider
	GohaiEndpoint apihelper.AgentEndpointProvider
}

// MockHost is a mocked struct that implements the host component interface
type MockHost struct{}

// GetPayloadAsJSON is a mocked method that we can use for testing
func (h *MockHost) GetPayloadAsJSON(_ context.Context) ([]byte, error) {
	str := "some bytes"
	return []byte(str), nil
}

// handlerFunc is a simple mocked http.Handler function
func (h *MockHost) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// newMock returns the mocked host component
func newMock() MockProvides {
	h := &MockHost{}
	return MockProvides{
		Comp:          h,
		Endpoint:      apihelper.NewAgentEndpointProvider(h.handlerFunc, "/metadata/v5", "GET"),
		GohaiEndpoint: apihelper.NewAgentEndpointProvider(h.handlerFunc, "/metadata/gohai", "GET"),
	}
}
