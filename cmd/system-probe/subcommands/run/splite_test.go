// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	"github.com/DataDog/datadog-agent/pkg/discovery/module/splite"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// createFakeSPLiteBinary creates a fake system-probe-lite and returns a lookup
// that locates system-probe in the same temporary directory.
func createFakeSPLiteBinary(t *testing.T) (string, spliteExecutableFunc) {
	t.Helper()
	execDir := t.TempDir()
	fakeBinary := filepath.Join(execDir, "system-probe-lite")
	require.NoError(t, os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0755))
	return fakeBinary, func() (string, error) {
		return filepath.Join(execDir, "system-probe"), nil
	}
}

// newMockSysprobeConfig creates a sysprobeconfig mock with overrides applied
// before the config is loaded, so SysProbeObject() reflects them.
func newMockSysprobeConfig(t *testing.T, overrides map[string]interface{}) sysprobeconfig.Component {
	return sysprobeconfigmock.NewMockWithOverrides(t, overrides)
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
			var executableFn spliteExecutableFunc
			if tc.fakeBinary {
				fakeBinaryPath, executableFn = createFakeSPLiteBinary(t)
			} else {
				execDir := t.TempDir()
				executableFn = func() (string, error) {
					return filepath.Join(execDir, "system-probe"), nil
				}
			}

			sysprobeConfig := newMockSysprobeConfig(t, tc.overrides)
			log := logmock.New(t)
			cmd := maybeSPLite(sysprobeConfig, "/test/sp.pid", log, executableFn)

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

func TestRunCommandExecsSPLiteBeforeLifecycleStart(t *testing.T) {
	configPath := prepareRunCommandTest(t)
	fakeBinaryPath, executableFn := createFakeSPLiteBinary(t)
	pidFilePath := filepath.Join(t.TempDir(), "system-probe.pid")
	t.Setenv("DD_DISCOVERY_ENABLED", "true")
	t.Setenv("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "true")

	var events []string
	execFn := spliteExecFunc(func(path string, args []string, env []string) error {
		events = append(events, "exec")
		require.FileExists(t, pidFilePath)
		assert.Equal(t, fakeBinaryPath, path)
		assert.Equal(t, fakeBinaryPath, args[0])
		assert.NotEmpty(t, env)
		return errors.New("test exec failure")
	})

	fxutil.TestOneShotSubcommand(t,
		commands(
			&command.GlobalParams{ConfFilePath: configPath},
			execFn,
			executableFn,
		),
		[]string{"run", "--pid", pidFilePath},
		run,
		func(lc fx.Lifecycle) {
			lc.Append(fx.Hook{OnStart: func(context.Context) error {
				events = append(events, "start")
				return nil
			}})
		},
	)

	require.Equal(t, []string{"exec", "start"}, events)
}
