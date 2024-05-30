// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package packagesigningimpl

import (
	"net/http"

	"go.uber.org/fx"

	apihelper "github.com/DataDog/datadog-agent/comp/api/api/helpers"
	psinterface "github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
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

	Comp     psinterface.Component
	Endpoint apihelper.AgentEndpointProvider
}

// MockPkgSigning is the mocked struct that implements the packagesigning component interface
type MockPkgSigning struct{}

// GetAsJSON is a mocked method on the component
func (ps *MockPkgSigning) GetAsJSON() ([]byte, error) {
	str := "some bytes"
	return []byte(str), nil
}

// handlerFunc is a simple mocked http.Handler function
func (ps *MockPkgSigning) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// newMock returns the mocked packagesigning struct
func newMock() MockProvides {
	ps := &MockPkgSigning{}
	return MockProvides{
		Comp:     ps,
		Endpoint: apihelper.NewAgentEndpointProvider(ps.handlerFunc, "/metadata/package-signing", "GET"),
	}
}
