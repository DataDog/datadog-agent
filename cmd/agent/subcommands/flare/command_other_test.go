// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !windows

// Package flare implements 'agent flare'.
package flare

import (
	"net/http"
	"net/http/httptest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

// NewSystemProbeTestServer starts a new mock server to handle System Probe requests.
func NewSystemProbeTestServer(_ http.Handler) (*httptest.Server, error) {
	// Linux still uses a port-based system-probe, it does not need a dedicated server
	// for the tests.
	return nil, nil
}

// RestartSystemProbeTestServer restarts the system probe server to ensure no cache responses
// are used for a test.
func RestartSystemProbeTestServer(_ *commandTestSuite) {
}

// InjectConnectionFailures injects a failure in TestReadProfileDataErrors.
func InjectConnectionFailures(_ model.Config, _ model.Config) {
}

// ClearConnectionFailures clears the injected failure in TestReadProfileDataErrors.
func ClearConnectionFailures() {
}

// CheckExpectedConnectionFailures checks the expected errors after simulated
// connection failures.
func CheckExpectedConnectionFailures(c *commandTestSuite, err error) {
	// System probe by default is disabled and no connection is attempted for it in the test.
	require.Regexp(c.T(), "^4 errors occurred:\n", err.Error())
}
