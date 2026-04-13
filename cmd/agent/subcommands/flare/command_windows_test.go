// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package flare implements 'agent flare'.
package flare

import (
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// InjectConnectionFailures injects a failure in TestReadProfileDataErrors.
func InjectConnectionFailures(mockSysProbeConfig model.Config, mockConfig model.Config) {
	// Explicitly enabled system probe to exercise connections to it.
	mockSysProbeConfig.SetInTest("system_probe_config.enabled", true)

	// Exercise a connection failure for a Windows system probe named pipe client by
	// making them use a bad path.
	// The system probe http server must be setup before this override.
	mockSysProbeConfig.SetInTest("system_probe_config.sysprobe_socket", `\\.\pipe\dd_system_probe_test_bad`)

	// The security-agent connection is expected to fail too in this test, but
	// by enabling system probe, a port will be provided to it (security agent).
	// Here we make sure the security agent port is a bad one.
	mockConfig.SetInTest("security_agent.expvar_port", 0)
}

// CheckExpectedConnectionFailures checks the expected errors after simulated
// connection failures.
func CheckExpectedConnectionFailures(c *commandTestSuite, err error) {
	// In Windows, this test explicitly simulates a system probe connection failure.
	// We expect the standard socket errors (4) and a named pipe failure for system probe.
	require.Regexp(c.T(), "^5 errors occurred:\n", err.Error())
}
