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

	"github.com/stretchr/testify/require"

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

// RestartSystemProbeTestServer restarts the system probe server to ensure no cache responses
// are used for a test.
func RestartSystemProbeTestServer(c *commandTestSuite) {
	// In Windows, the http server caches responses from prior test runs.
	// This prevents simulating connection failures.
	// Thus on every test run, we need to recreate the http server.
	if c.systemProbeServer != nil {
		c.systemProbeServer.Close()
		c.systemProbeServer = nil
	}

	var err error
	c.systemProbeServer, err = NewSystemProbeTestServer(newMockHandler())
	require.NoError(c.T(), err, "could not restart system probe server")
	c.systemProbeServer.Start()
}

// InjectConnectionFailures injects a failure in TestReadProfileDataErrors.
func InjectConnectionFailures(mockSysProbeConfig model.Config, mockConfig model.Config) {
	// The system probe http server must be setup before this.
	// Exercise a connection failure for the Windows system-probe named pipe by overriding
	// the path with a bad one before the named pipe client creates the connection.
	mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
	process_net.SystemProbePipeName = `\\.\pipe\dd_system_probe_test_bad`

	// The security-agent connection is expected to fail too in this test, but
	// by enabling system probe, a port will be provided to it (security agent).
	// Here we make sure the security agent port is a bad one.
	mockConfig.SetWithoutSource("security_agent.expvar_port", 0)
}

// ClearConnectionFailures clears the injected failure in TestReadProfileDataErrors.
func ClearConnectionFailures() {
	// Disable system probe and restore the named pipe path.
	process_net.SystemProbePipeName = SystemProbeTestPipeName
}

// CheckExpectedConnectionFailures checks the expected errors after simulated
// connection failures.
func CheckExpectedConnectionFailures(c *commandTestSuite, err error) {
	// In Windows, this test explicitly simulates a system probe connection failure.
	// We expect the standard socket errors (4) and a named pipe failure for system probe.
	require.Regexp(c.T(), "^5 errors occurred:\n", err.Error())
}
