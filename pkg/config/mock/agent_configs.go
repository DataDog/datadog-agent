// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package mock

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// InitAgentConfigs mocks both the datadog and system-probe configurations, for tests that load real configuration files
// into the globals through the config components. It fails if either is already mocked in the test. The environment
// variables are read while the configurations are built, so they must be set before.
func InitAgentConfigs(t testing.TB) {
	m.Lock()
	alreadyMocked := isConfigMocked || isSystemProbeConfigMocked
	m.Unlock()
	require.False(t, alreadyMocked, "the configurations are already mocked in this test")

	New(t)
	NewSystemProbe(t)
}
