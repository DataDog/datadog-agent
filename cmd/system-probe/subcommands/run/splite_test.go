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
	"go.uber.org/fx"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/discovery/module/splite"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// createFakeSPLiteBinary creates a fake system-probe-lite binary next to the
// test binary and returns cleanup func.
func createFakeSPLiteBinary(t *testing.T) string {
	t.Helper()
	execPath, err := os.Executable()
	require.NoError(t, err)
	fakeBinary := filepath.Join(filepath.Dir(execPath), "system-probe-lite")
	require.NoError(t, os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0755))
	t.Cleanup(func() { os.Remove(fakeBinary) })
	return fakeBinary
}

// newMockSysprobeConfig creates a sysprobeconfig mock with overrides applied
// before the config is loaded, so SysProbeObject() reflects them.
func newMockSysprobeConfig(t *testing.T, overrides map[string]interface{}) sysprobeconfig.Component {
	return fxutil.Test[sysprobeconfig.Component](t,
		sysprobeconfigimpl.MockModule(),
		fx.Replace(sysprobeconfigimpl.MockParams{Overrides: overrides}),
	)
}

func TestMaybeSPLite(t *testing.T) {
	tests := []struct {
		name       string
		overrides  map[string]interface{}
		fakeBinary bool
		expectNil  bool
	}{
		{
			name: "feature disabled",
			overrides: map[string]interface{}{
				"discovery.use_system_probe_lite": false,
				"discovery.enabled":               true,
			},
			fakeBinary: true,
			expectNil:  true,
		},
		{
			name: "only discovery module",
			overrides: map[string]interface{}{
				"discovery.use_system_probe_lite": true,
				"discovery.enabled":               true,
			},
			fakeBinary: true,
			expectNil:  false,
		},
		{
			name: "multiple modules",
			overrides: map[string]interface{}{
				"discovery.use_system_probe_lite": true,
				"discovery.enabled":               true,
				"network_config.enabled":          true,
			},
			fakeBinary: true,
			expectNil:  true,
		},
		{
			name: "external system-probe",
			overrides: map[string]interface{}{
				"discovery.use_system_probe_lite": true,
				"discovery.enabled":               true,
				"system_probe_config.external":    true,
			},
			fakeBinary: true,
			expectNil:  true,
		},
		{
			name: "discovery explicitly disabled",
			overrides: map[string]interface{}{
				"discovery.use_system_probe_lite": true,
				"discovery.enabled":               false,
			},
			fakeBinary: true,
			expectNil:  true,
		},
		{
			name: "binary not found",
			overrides: map[string]interface{}{
				"discovery.use_system_probe_lite": true,
				"discovery.enabled":               true,
			},
			fakeBinary: false,
			expectNil:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var fakeBinaryPath string
			if tc.fakeBinary {
				fakeBinaryPath = createFakeSPLiteBinary(t)
			}

			sysprobeConfig := newMockSysprobeConfig(t, tc.overrides)
			log := logmock.New(t)
			cmd := maybeSPLite(sysprobeConfig, "/test/sp.pid", log)

			if tc.expectNil {
				assert.Nil(t, cmd)
				return
			}

			require.NotNil(t, cmd)
			assert.Equal(t, fakeBinaryPath, cmd.Path)
			assert.Equal(t, fakeBinaryPath, cmd.Args[0])

			// Verify args match what the splite package produces (source of truth)
			expectedArgs := (&splite.Config{
				Socket:   sysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
				LogLevel: sysprobeConfig.GetString("log_level"),
				LogFile:  sysprobeConfig.GetString("log_file"),
				PIDFile:  "/test/sp.pid",
			}).Args()
			assert.Equal(t, expectedArgs, cmd.Args[1:])
			assert.NotEmpty(t, cmd.Env)
		})
	}
}
