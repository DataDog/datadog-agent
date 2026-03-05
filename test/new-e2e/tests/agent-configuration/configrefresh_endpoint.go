// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentconfiguration

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
)

// agentConfigEndpointInfo holds connection details for an agent's config HTTP endpoint.
type agentConfigEndpointInfo struct {
	name     string
	scheme   string
	port     int
	endpoint string
}

// traceConfigEndpoint returns the endpoint info for the trace-agent config endpoint.
func traceConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"trace-agent", "https", port, "/config"}
}

// processConfigEndpoint returns the endpoint info for the process-agent config endpoint.
func processConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"process-agent", "https", port, "/config/all"}
}

// securityConfigEndpoint returns the endpoint info for the security-agent config endpoint.
func securityConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"security-agent", "https", port, "/agent/config"}
}

// url builds the full URL for this endpoint.
func (endpointInfo *agentConfigEndpointInfo) url() *url.URL {
	return &url.URL{
		Scheme: endpointInfo.scheme,
		Host:   net.JoinHostPort("localhost", strconv.Itoa(endpointInfo.port)),
		Path:   endpointInfo.endpoint,
	}
}

// httpRequest creates an authenticated GET request for this endpoint.
func (endpointInfo *agentConfigEndpointInfo) httpRequest(authtoken string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, endpointInfo.url().String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+authtoken)
	return req, nil
}
