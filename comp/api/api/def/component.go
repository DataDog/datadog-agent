// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package def implements the internal Agent API component definitions which exposes endpoints such as config, flare or status
package def

import (
	"net"
	"net/http"

	"go.uber.org/fx"
)

// team: agent-runtimes

// TODO(components):
// * Lifecycle can't be used atm because:
//     - logsAgent and remoteconfig.Service are modified in `startAgent` in the run subcommand
//     - Same for workloadmeta and senderManager in `execJmxCommand` in the jmx subcommand

// Component is the component type.
type Component interface {
	CMDServerAddress() *net.TCPAddr
	IPCServerAddress() *net.TCPAddr
}

// EndpointProvider is an interface to register api endpoints
type EndpointProvider interface {
	HandlerFunc() http.HandlerFunc

	Methods() []string
	Route() string
}

// endpointProvider is the implementation of EndpointProvider interface
type endpointProvider struct {
	methods []string
	route   string
	handler http.HandlerFunc
}

// AuthorizedSet is a type to store the authorized config options for the config API
type AuthorizedSet map[string]struct{}

// AuthorizedConfigPathsCore is the the set of authorized config keys authorized for the
// config API.
var AuthorizedConfigPathsCore = buildAuthorizedSet(
	"api_key", "site", "dd_url", "logs_config.dd_url",
	"additional_endpoints", "logs_config.additional_endpoints", "apm_config.additional_endpoints",
)

func buildAuthorizedSet(paths ...string) AuthorizedSet {
	authorizedPaths := make(AuthorizedSet, len(paths))
	for _, path := range paths {
		authorizedPaths[path] = struct{}{}
	}
	return authorizedPaths
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

	Provider EndpointProvider `group:"agent_endpoint"`
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
