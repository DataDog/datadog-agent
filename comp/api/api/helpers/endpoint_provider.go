// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the healthprobe component
package helpers

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/api/def"
)

// endpointProvider is the implementation of EndpointProvider interface
type endpointProvider struct {
	methods []string
	route   string
	handler http.HandlerFunc
}

// Methods returns the methods for the endpoint.
// e.g.: "GET", "POST", "PUT".
func (p endpointProvider) Methods() []string {
	return p.methods
}

// Route returns the route for the endpoint.
func (p endpointProvider) Route() string {
	return p.route
}

// HandlerFunc returns the handler function for the endpoint.
func (p endpointProvider) HandlerFunc() http.HandlerFunc {
	return p.handler
}

// AgentEndpointProvider is the provider for registering endpoints to the internal agent api server
type AgentEndpointProvider struct {
	fx.Out

	Provider def.EndpointProvider `group:"agent_endpoint"`
}

// NewAgentEndpointProvider returns a AgentEndpointProvider to register the endpoint provided to the internal agent api server
func NewAgentEndpointProvider(handlerFunc http.HandlerFunc, route string, methods ...string) AgentEndpointProvider {
	return AgentEndpointProvider{
		Provider: endpointProvider{
			handler: handlerFunc,
			route:   route,
			methods: methods,
		},
	}
}
