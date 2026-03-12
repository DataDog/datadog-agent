// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

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
	"github.com/DataDog/datadog-agent/pkg/config/model"
	systemprobeconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func TestShouldExecSPLite(t *testing.T) {
	tests := []struct {
		name                string
		useSystemProbeLite  bool
		discoveryEnabled    *bool // nil = not configured
		externalSystemProbe bool
		enabledModules      map[sysconfigtypes.ModuleName]struct{}
		enabled             bool
		expected            bool
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
			useSystemProbeLite:  true,
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
			assert.Equal(t, tc.expected, shouldExecSPLite(sysprobeConfig, cfg))
		})
	}
}

func TestBuildSPLiteArgs(t *testing.T) {
	sysprobeConfig := sysprobeconfigimpl.NewMock(t)
	sysprobeConfig.Set("system_probe_config.sysprobe_socket", "/custom/path.sock", model.SourceCLI)
	sysprobeConfig.Set("log_level", "debug", model.SourceCLI)

	args := buildSPLiteArgs(sysprobeConfig, "/var/run/sp.pid")

	assert.Equal(t, "system-probe-lite", args[0])
	assert.Contains(t, args, "/custom/path.sock")
	assert.Contains(t, args, "debug")
	assert.Contains(t, args, sysprobeConfig.GetString("log_file"))
	assert.Contains(t, args, "/var/run/sp.pid")
}

func TestResolveSPLiteExecCmd(t *testing.T) {
	t.Run("returns nil when system-probe-lite binary not found", func(t *testing.T) {
		sysprobeConfig := sysprobeconfigimpl.NewMock(t)
		log := logmock.New(t)

		// The test binary's directory won't have a system-probe-lite binary
		cmd := resolveSPLiteExecCmd(sysprobeConfig, "/var/run/sp.pid", log)
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

		cmd := resolveSPLiteExecCmd(sysprobeConfig, "/var/run/sp.pid", log)
		require.NotNil(t, cmd, "should return exec cmd when system-probe-lite binary exists")
		assert.Equal(t, testBinary, cmd.Path)
		assert.Equal(t, []string{
			"system-probe-lite",
			"--socket", sysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
			"--log-level", sysprobeConfig.GetString("log_level"),
			"--log-file", sysprobeConfig.GetString("log_file"),
			"--pid", "/var/run/sp.pid",
		}, cmd.Args)
		assert.NotEmpty(t, cmd.Env)
	})

	t.Run("graceful fallback for maybeSPLite", func(t *testing.T) {
		sysprobeConfig := sysprobeconfigimpl.NewMock(t)
		sysprobeConfig.SetWithoutSource("discovery.use_system_probe_lite", true)
		log := logmock.New(t)

		// maybeSPLite should return (not panic) when binary is missing,
		// allowing system-probe to fall back to the Go discovery module.
		maybeSPLite(sysprobeConfig, "/var/run/sp.pid", log)
	})
}

func boolPtr(b bool) *bool {
	return &b
}
