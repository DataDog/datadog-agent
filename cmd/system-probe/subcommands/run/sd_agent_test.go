// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	systemprobeconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func TestShouldExecSystemProbeLite(t *testing.T) {
	tests := []struct {
		name                   string
		useSystemProbeLite     bool
		discoveryEnabled       *bool // nil = not configured
		externalSystemProbe    bool
		enabledModules         map[sysconfigtypes.ModuleName]struct{}
		enabled                bool
		expected               bool
	}{
		{
			name:               "use_system_probe_lite disabled (default)",
			useSystemProbeLite: false,
			enabledModules:     map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
			enabled:            true,
			expected:           false,
		},
		{
			name:               "only discovery module enabled",
			useSystemProbeLite: true,
			enabledModules:     map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
			enabled:            true,
			expected:           true,
		},
		{
			name:               "no modules enabled",
			useSystemProbeLite: true,
			enabledModules:     map[sysconfigtypes.ModuleName]struct{}{},
			enabled:            false,
			expected:           true,
		},
		{
			name:               "multiple modules enabled",
			useSystemProbeLite: true,
			enabledModules: map[sysconfigtypes.ModuleName]struct{}{
				systemprobeconfig.DiscoveryModule:     {},
				systemprobeconfig.NetworkTracerModule: {},
			},
			enabled:  true,
			expected: false,
		},
		{
			name:               "only non-discovery module enabled",
			useSystemProbeLite: true,
			enabledModules: map[sysconfigtypes.ModuleName]struct{}{
				systemprobeconfig.NetworkTracerModule: {},
			},
			enabled:  true,
			expected: false,
		},
		{
			name:                "external system-probe",
			useSystemProbeLite: true,
			externalSystemProbe: true,
			enabledModules:      map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
			enabled:             true,
			expected:            false,
		},
		{
			name:               "discovery explicitly disabled, nothing else enabled",
			useSystemProbeLite: true,
			discoveryEnabled:   boolPtr(false),
			enabledModules:     map[sysconfigtypes.ModuleName]struct{}{},
			enabled:            false,
			expected:           false,
		},
		{
			name:               "discovery explicitly enabled",
			useSystemProbeLite: true,
			discoveryEnabled:   boolPtr(true),
			enabledModules:     map[sysconfigtypes.ModuleName]struct{}{systemprobeconfig.DiscoveryModule: {}},
			enabled:            true,
			expected:           true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sysprobeConfig := sysprobeconfigimpl.NewMock(t)
			sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", tc.useSystemProbeLite)
			if tc.discoveryEnabled != nil {
				sysprobeConfig.SetWithoutSource("discovery.enabled", *tc.discoveryEnabled)
			}

			cfg := &sysconfigtypes.Config{
				Enabled:             tc.enabled,
				EnabledModules:      tc.enabledModules,
				ExternalSystemProbe: tc.externalSystemProbe,
			}
			assert.Equal(t, tc.expected, shouldExecSystemProbeLite(sysprobeConfig, cfg))
		})
	}
}

func TestBuildSystemProbeLiteArgs(t *testing.T) {
	t.Run("no config no pid", func(t *testing.T) {
		sysprobeConfig := sysprobeconfigimpl.NewMock(t)
		args := buildSystemProbeLiteArgs(sysprobeConfig, "")
		assert.Equal(t, []string{"system-probe-lite", "run"}, args)
	})

	t.Run("with pid only", func(t *testing.T) {
		sysprobeConfig := sysprobeconfigimpl.NewMock(t)
		args := buildSystemProbeLiteArgs(sysprobeConfig, "/opt/datadog-agent/run/system-probe.pid")
		assert.Equal(t, []string{"system-probe-lite", "run", "--pid", "/opt/datadog-agent/run/system-probe.pid"}, args)
	})
}

func TestResolveSystemProbeLiteExecCmd(t *testing.T) {
	t.Run("returns nil when system-probe-lite binary not found", func(t *testing.T) {
		sysprobeConfig := sysprobeconfigimpl.NewMock(t)
		log := logmock.New(t)

		// The test binary's directory won't have a system-probe-lite binary
		cmd := resolveSystemProbeLiteExecCmd(sysprobeConfig, "/var/run/sp.pid", log)
		assert.Nil(t, cmd, "should return nil when system-probe-lite binary is not found next to the test binary")
	})

	t.Run("returns valid command when system-probe-lite binary exists", func(t *testing.T) {
		// Place a fake system-probe-lite binary next to the test binary
		execPath, err := os.Executable()
		require.NoError(t, err)
		testBinary := filepath.Join(filepath.Dir(execPath), "system-probe-lite")
		require.NoError(t, os.WriteFile(testBinary, []byte("#!/bin/sh\n"), 0755))
		t.Cleanup(func() { os.Remove(testBinary) })

		sysprobeConfig := sysprobeconfigimpl.NewMock(t)
		log := logmock.New(t)

		cmd := resolveSystemProbeLiteExecCmd(sysprobeConfig, "/var/run/sp.pid", log)
		require.NotNil(t, cmd, "should return exec cmd when system-probe-lite binary exists")
		assert.Equal(t, testBinary, cmd.Path)
		assert.Equal(t, []string{"system-probe-lite", "run", "--pid", "/var/run/sp.pid"}, cmd.Args)
		assert.NotEmpty(t, cmd.Env)
	})

	t.Run("graceful fallback returns nil for execSystemProbeLite", func(t *testing.T) {
		sysprobeConfig := sysprobeconfigimpl.NewMock(t)
		log := logmock.New(t)

		// execSystemProbeLite should return nil (not an error) when binary is missing,
		// allowing system-probe to fall back to the Go discovery module.
		err := execSystemProbeLite(sysprobeConfig, "/var/run/sp.pid", log)
		assert.NoError(t, err, "execSystemProbeLite should return nil when system-probe-lite is not found")
	})
}

func boolPtr(b bool) *bool {
	return &b
}
