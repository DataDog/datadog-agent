// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package demultiplexerendpointimpl

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for this component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
	)
}

type mock struct {
}

// ServeHTTP is a simple mocked http.Handler function
func (m *mock) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Endpoint api.AgentEndpointProvider
}

func newMock() MockProvides {

	instance := &mock{}
	return MockProvides{
		Endpoint: api.NewAgentEndpointProvider(instance.handlerFunc, "/dogstatsd-contexts-dump", "POST"),
	}
}
