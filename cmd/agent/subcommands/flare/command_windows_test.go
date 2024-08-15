// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package flare implements 'agent flare'.
package flare

import (
	"net/http"
	"net/http/httptest"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	process_net "github.com/DataDog/datadog-agent/pkg/process/net"
)

const (
	// SystemProbeTestPipeName is the test named pipe for system-probe
	SystemProbeTestPipeName = `\\.\pipe\dd_system_probe_test`
)

// NewSystemProbeTestServer starts a new mock server to handle System Probe requests.
func NewSystemProbeTestServer(handler http.Handler) (*httptest.Server, error) {
	// Override the named pipe path for test to avoid conflicts with the production
	// named pipe (in case the agent is installed locally).
	process_net.SystemProbePipeName = SystemProbeTestPipeName

	server := httptest.NewUnstartedServer(handler)

	// The Windows System Probe uses a named pipe, it does not care about ports.
	conn, err := process_net.NewSystemProbeListener("")
	if err != nil {
		return nil, err
	}

	server.Listener = conn.GetListener()
	return server, nil
}

// InjectConnectionFailures injects a failure in TestReadProfileDataErrors.
func InjectConnectionFailures(mockSysProbeConfig model.Config, mockConfig model.Config) {
	// The system probe http server must be setup before this.
	// Override the named pipe path for the client and enable system probe to try make
	// a connection
	mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
	process_net.SystemProbePipeName = `\\.\pipe\dd_system_probe_test_bad`

	// Make the security-agent connection fail too.
	mockConfig.SetWithoutSource("security_agent.expvar_port", 0)
}

// ClearConnectionFailures clears the injected failure in TestReadProfileDataErrors.
func ClearConnectionFailures(_ model.Config, _ model.Config) {
	// Disable system probe and restore the named pipe path.
	process_net.SystemProbePipeName = SystemProbeTestPipeName
}
