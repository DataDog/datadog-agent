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
	"go.uber.org/fx/fxtest"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	delegatedauthnoopfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pidfx "github.com/DataDog/datadog-agent/comp/core/pid/fx"
	pidimpl "github.com/DataDog/datadog-agent/comp/core/pid/impl"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigfx "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/fx"
	sysprobeconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/impl"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
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

func TestEarlySPLiteExecFailureFallsThroughToLifecycleStart(t *testing.T) {
	fakeBinaryPath, executableFn := createFakeSPLiteBinary(t)
	pidFilePath := filepath.Join(t.TempDir(), "system-probe.pid")
	sysprobeConfig := newMockSysprobeConfig(t, map[string]interface{}{
		"discovery.use_system_probe_lite": true,
		"discovery.enabled":               true,
	})
	logger := logmock.New(t)

	var events []string
	execFn := spliteExecFunc(func(path string, args []string, env []string) error {
		events = append(events, "exec")
		require.FileExists(t, pidFilePath)
		assert.Equal(t, fakeBinaryPath, path)
		assert.Equal(t, fakeBinaryPath, args[0])
		assert.NotEmpty(t, env)
		return errors.New("test exec failure")
	})

	app := fxtest.New(t,
		fx.NopLogger,
		fxutil.FxAgentBase(),
		fx.Provide(func() sysprobeconfig.Component { return sysprobeConfig }),
		fx.Provide(func() log.Component { return logger }),
		fx.Supply(pidimpl.NewParams(pidFilePath)),
		pidfx.Module(),
		fx.Supply(executableFn),
		fx.Supply(execFn),
		fx.Invoke(tryExecSPLiteEarly),
		fx.Invoke(func(lc fx.Lifecycle) {
			lc.Append(fx.Hook{OnStart: func(context.Context) error {
				events = append(events, "start")
				return nil
			}})
		}),
	)

	assert.Equal(t, []string{"exec"}, events)
	app.RequireStart()
	assert.Equal(t, []string{"exec", "start"}, events)
	app.RequireStop()
	require.NoFileExists(t, pidFilePath)
}

type activeConfigStream struct{}

func (activeConfigStream) IsActive() bool { return true }

func TestEarlySPLiteHandoffUsesStreamedCoreConfig(t *testing.T) {
	_, executableFn := createFakeSPLiteBinary(t)
	t.Setenv("DD_DISCOVERY_ENABLED", "true")
	t.Setenv("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "true")

	configParams := config.NewAgentParams("")
	t.Cleanup(pkgconfigsetup.InitConfigObjects)
	configstreambootstrap.UseDynamicSchema(t)
	streamedConfig := configstreambootstrap.Config()
	streamedConfig.Set("compliance_config.enabled", true, model.SourceFile)
	streamedConfig.Set("compliance_config.run_in_system_probe", true, model.SourceFile)

	execCalled := false
	app := fxtest.New(t,
		fx.NopLogger,
		fxutil.FxAgentBase(),
		fx.Supply(configParams),
		fx.Supply(sysprobeconfigimpl.NewParams()),
		fx.Supply(pidimpl.NewParams("")),
		pidfx.Module(),
		fx.Supply(executableFn),
		fx.Supply(spliteExecFunc(func(string, []string, []string) error {
			execCalled = true
			return errors.New("unexpected exec")
		})),
		fx.Provide(func() configstreamconsumer.Component { return activeConfigStream{} }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.Module(),
		delegatedauthnoopfx.Module(),
		secretsnoopfx.Module(),
		sysprobeconfigfx.Module(),
		fx.Invoke(tryExecSPLiteEarly),
	)

	assert.False(t, execCalled, "streamed config enables compliance, so full system-probe is required")
	app.RequireStart().RequireStop()
}
