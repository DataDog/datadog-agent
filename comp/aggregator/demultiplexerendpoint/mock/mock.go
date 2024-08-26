// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package demultiplexerendpointmock provides a mock component for demultiplexerendpoint
package demultiplexerendpointmock

import (
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
)

type mock struct {
}

// ServeHTTP is a simple mocked http.Handler function
func (m *mock) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// Provides is the mock component output
type Provides struct {
	Endpoint api.AgentEndpointProvider
}

// NewMock creates a new mock component
func NewMock() Provides {
	instance := &mock{}
	return Provides{
		Endpoint: api.NewAgentEndpointProvider(instance.handlerFunc, "/dogstatsd-contexts-dump", "POST"),
	}
}
