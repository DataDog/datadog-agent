// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
)

// AgentConfigEndpointInfo holds connection details for an agent's config HTTP endpoint.
type AgentConfigEndpointInfo struct {
	Name     string
	scheme   string
	port     int
	endpoint string
}

// TraceConfigEndpoint returns the endpoint info for the trace-agent config endpoint.
func TraceConfigEndpoint(port int) AgentConfigEndpointInfo {
	return AgentConfigEndpointInfo{"trace-agent", "https", port, "/config"}
}

// ProcessConfigEndpoint returns the endpoint info for the process-agent config endpoint.
func ProcessConfigEndpoint(port int) AgentConfigEndpointInfo {
	return AgentConfigEndpointInfo{"process-agent", "https", port, "/config/all"}
}

// SecurityConfigEndpoint returns the endpoint info for the security-agent config endpoint.
func SecurityConfigEndpoint(port int) AgentConfigEndpointInfo {
	return AgentConfigEndpointInfo{"security-agent", "https", port, "/agent/config"}
}

// URL builds the full URL for this endpoint.
func (endpointInfo *AgentConfigEndpointInfo) URL() *url.URL {
	return &url.URL{
		Scheme: endpointInfo.scheme,
		Host:   net.JoinHostPort("localhost", strconv.Itoa(endpointInfo.port)),
		Path:   endpointInfo.endpoint,
	}
}

// HTTPRequest creates an authenticated GET request for this endpoint.
func (endpointInfo *AgentConfigEndpointInfo) HTTPRequest(authtoken string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, endpointInfo.URL().String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+authtoken)
	return req, nil
}
