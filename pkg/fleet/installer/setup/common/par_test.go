// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
)

func TestPARInstallerDefault(t *testing.T) {
	for _, tt := range []struct {
		name       string
		goos       string
		procmgr    bool
		systemd    bool
		fips       bool
		noAgent    bool
		envValue   string
		existing   string
		configured *bool
		want       *bool
	}{
		{name: "fresh systemd install", goos: "linux", procmgr: true, systemd: true, want: config.BoolToPtr(true)},
		{name: "reinstall with unset key", goos: "linux", procmgr: true, systemd: true, existing: "api_key: test\n", want: config.BoolToPtr(true)},
		{name: "preserve explicit YAML opt out", goos: "linux", procmgr: true, systemd: true, existing: "private_action_runner:\n  enabled: false\n"},
		{name: "preserve explicit YAML opt in", goos: "linux", procmgr: true, systemd: true, existing: "private_action_runner:\n  enabled: true\n"},
		{name: "preserve quoted YAML opt out", goos: "linux", procmgr: true, systemd: true, existing: "private_action_runner:\n  enabled: \"false\"\n"},
		{name: "preserve numeric YAML opt out", goos: "linux", procmgr: true, systemd: true, existing: "private_action_runner:\n  enabled: 0\n"},
		{name: "preserve null YAML setting", goos: "linux", procmgr: true, systemd: true, existing: "private_action_runner:\n  enabled: null\n"},
		{name: "installer opt out", goos: "linux", procmgr: true, systemd: true, envValue: "false", configured: config.BoolToPtr(false), want: config.BoolToPtr(false)},
		{name: "invalid environment value does not opt in", goos: "linux", procmgr: true, systemd: true, envValue: "invalid"},
		{name: "programmatic opt out", goos: "linux", procmgr: true, systemd: true, configured: config.BoolToPtr(false), want: config.BoolToPtr(false)},
		{name: "procmgr disabled", goos: "linux", systemd: true},
		{name: "sysv upstart or container without systemd", goos: "linux", procmgr: true},
		{name: "FIPS split mode unsupported", goos: "linux", procmgr: true, systemd: true, fips: true},
		{name: "Windows unchanged", goos: "windows", procmgr: true, systemd: true},
		{name: "macOS unchanged", goos: "darwin", procmgr: true, systemd: true},
		{name: "SSI only install", goos: "linux", procmgr: true, systemd: true, noAgent: true},
		{name: "explicit legacy opt in preserved", goos: "linux", configured: config.BoolToPtr(true), want: config.BoolToPtr(true)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", tt.envValue)
			s := &Setup{configDir: t.TempDir(), Env: &env.Env{ProcessManagerEnabled: tt.procmgr, FIPSMode: tt.fips}}
			s.Config.DatadogYAML.PrivateActionRunner.Enabled = tt.configured
			if tt.existing != "" {
				require.NoError(t, os.WriteFile(filepath.Join(s.configDir, "datadog.yaml"), []byte(tt.existing), 0600))
			}
			packages := []packageWithVersion{{name: DatadogAgentPackage}}
			if tt.noAgent {
				packages = []packageWithVersion{{name: DatadogAPMInjectPackage}}
			}
			require.NoError(t, s.setPARDefault(packages, tt.goos, func() (bool, error) { return tt.systemd, nil }))
			assert.Equal(t, tt.want, s.Config.DatadogYAML.PrivateActionRunner.Enabled)
			assert.Nil(t, s.Config.DatadogYAML.PrivateActionRunner.SelfEnroll)
			assert.Nil(t, s.Config.DatadogYAML.PrivateActionRunner.ActionsAllowlist)
		})
	}
}

func TestPARInstallerDefaultPreservesOtherConfiguration(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "")
	s := &Setup{configDir: t.TempDir(), Env: &env.Env{ProcessManagerEnabled: true}}
	path := filepath.Join(s.configDir, "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte("api_key: test\nprivate_action_runner:\n  self_enroll: false\n  actions_allowlist:\n    - custom.action\n"), 0600))
	require.NoError(t, s.setPARDefault([]packageWithVersion{{name: DatadogAgentPackage}}, "linux", func() (bool, error) { return true, nil }))
	require.NoError(t, config.WriteConfigs(s.Config, s.configDir))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "enabled: true")
	assert.Contains(t, string(data), "self_enroll: false")
	assert.Contains(t, string(data), "custom.action")
	assert.Contains(t, string(data), "api_key: test")
}

func TestPARInstallerDefaultErrors(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "")
	s := &Setup{configDir: t.TempDir(), Env: &env.Env{ProcessManagerEnabled: true}}
	packages := []packageWithVersion{{name: DatadogAgentPackage}}
	err := s.setPARDefault(packages, "linux", func() (bool, error) { return false, errors.New("probe failed") })
	require.ErrorContains(t, err, "probe failed")
	assert.Nil(t, s.Config.DatadogYAML.PrivateActionRunner.Enabled)
	require.NoError(t, os.WriteFile(filepath.Join(s.configDir, "datadog.yaml"), []byte("private_action_runner: ["), 0600))
	err = s.setPARDefault(packages, "linux", func() (bool, error) { return true, nil })
	require.ErrorContains(t, err, "read existing PAR setting")
	assert.Nil(t, s.Config.DatadogYAML.PrivateActionRunner.Enabled)
}
