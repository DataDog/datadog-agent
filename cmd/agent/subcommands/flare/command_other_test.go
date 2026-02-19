// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !windows && !darwin

// Package flare implements 'agent flare'.
package flare

import (
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// InjectConnectionFailures injects a failure in TestReadProfileDataErrors.
func InjectConnectionFailures(mockSysProbeConfig model.Config, _ model.Config) {
	mockSysProbeConfig.SetInTest("system_probe_config.enabled", true)
	mockSysProbeConfig.SetInTest("system_probe_config.sysprobe_socket", "/opt/datadog-agent/run/sysprobe-bad.sock")
	mockSysProbeConfig.SetInTest("network_config.enabled", true)
}

// CheckExpectedConnectionFailures checks the expected errors after simulated
// connection failures.
func CheckExpectedConnectionFailures(c *commandTestSuite, err error) {
	require.Regexp(c.T(), "^5 errors occurred:\n", err.Error())
}
