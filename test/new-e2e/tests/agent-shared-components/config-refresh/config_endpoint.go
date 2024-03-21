// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configrefresh

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

type agentConfigEndpointInfo struct {
	name     string
	scheme   string
	port     int
	endpoint string
}

func traceConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"trace-agent", "http", port, "/config"}
}

func processConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"process-agent", "http", port, "/config/all"}
}

func securityConfigEndpoint(port int) agentConfigEndpointInfo {
	return agentConfigEndpointInfo{"security-agent", "https", port, "/agent/config"}
}

func (endpointInfo *agentConfigEndpointInfo) url() *url.URL {
	return &url.URL{
		Scheme: endpointInfo.scheme,
		Host:   net.JoinHostPort("localhost", strconv.Itoa(endpointInfo.port)),
		Path:   endpointInfo.endpoint,
	}
}

func (endpointInfo *agentConfigEndpointInfo) fetchCommand(authtoken string) string {
	// -L: follow redirects
	// -s: silent
	// -k: allow insecure server connections
	// -H: add a header
	return fmt.Sprintf(
		`curl -L -s -k -H "authorization: Bearer %s" "%s"`,
		authtoken,
		endpointInfo.url().String(),
	)
}
