// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !windows && !darwin

// Package flare implements 'agent flare'.
package flare

import (
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func sysprobeSocketPath(t *testing.T) string {
	return path.Join(t.TempDir(), "sysprobe.sock")
}

// NewSystemProbeTestServer starts a new mock server to handle System Probe requests.
func NewSystemProbeTestServer(_ http.Handler) (*httptest.Server, error) {
	// Linux still uses a port-based system-probe, it does not need a dedicated system probe server
	// for the tests.
	return nil, nil
}

// InjectConnectionFailures injects a failure in TestReadProfileDataErrors.
func InjectConnectionFailures(mockSysProbeConfig model.Config, _ model.Config) {
	mockSysProbeConfig.SetWithoutSource("system_probe_config.enabled", true)
	mockSysProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", "/opt/datadog-agent/run/sysprobe-bad.sock")
}

// CheckExpectedConnectionFailures checks the expected errors after simulated
// connection failures.
func CheckExpectedConnectionFailures(c *commandTestSuite, err error) {
	require.Regexp(c.T(), "^5 errors occurred:\n", err.Error())
}
