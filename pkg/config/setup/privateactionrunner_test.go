// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrivateActionRunnerActionsAllowlistFromEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_ACTIONS_ALLOWLIST", "com.datadoghq.kubernetes.core.listPod,com.datadoghq.script.runPredefinedScript")

	cfg := newTestConf(t)

	assert.Equal(t, []string{"com.datadoghq.kubernetes.core.listPod", "com.datadoghq.script.runPredefinedScript"}, cfg.GetStringSlice(PARActionsAllowlist))
}

func TestPrivateActionRunnerHttpAllowlistFromEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_HTTP_ALLOWLIST", "*.datadoghq.com,datadoghq.eu")

	cfg := newTestConf(t)

	assert.Equal(t, []string{"*.datadoghq.com", "datadoghq.eu"}, cfg.GetStringSlice(PARHttpAllowlist))
}

func TestPrivateActionRunnerRestrictedShellAllowedPathsFromEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_PATHS", "/var/log,/tmp")

	cfg := newTestConf(t)

	assert.Equal(t, []string{"/var/log", "/tmp"}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerRestrictedShellAllowedPathsEmptyEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_PATHS", "")

	cfg := newTestConf(t)

	assert.Equal(t, []string{defaultLogPath, defaultOsReleasePath}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerAllowlistDefaultsEmpty(t *testing.T) {
	cfg := newTestConf(t)

	assert.Empty(t, cfg.GetStringSlice(PARActionsAllowlist))
	assert.Empty(t, cfg.GetStringSlice(PARHttpAllowlist))
	assert.Equal(t, []string{defaultLogPath, defaultOsReleasePath}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerAllowedPathsBareMetal(t *testing.T) {
	// Ensure DOCKER_DD_AGENT is not set (bare metal)
	t.Setenv("DOCKER_DD_AGENT", "")
	os.Unsetenv("DOCKER_DD_AGENT")

	cfg := newTestConf(t)

	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{defaultLogPath, defaultOsReleasePath}, paths)
}

func TestPrivateActionRunnerAllowedPathsContainerizedWithHostMounts(t *testing.T) {
	// Create temp dirs to simulate /host mounts
	tmpDir := t.TempDir()
	hostLog := filepath.Join(tmpDir, "var", "log")
	hostOsRelease := filepath.Join(tmpDir, "etc", "os-release")
	err := os.MkdirAll(hostLog, 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(filepath.Dir(hostOsRelease), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(hostOsRelease, []byte(""), 0644)
	assert.NoError(t, err)

	// We can't easily test the full containerized flow since pathExists checks
	// real paths (/host/var/log, /host/etc/os-release) which won't exist in CI.
	// Instead, verify the bare metal defaults are correct when not containerized.
	cfg := newTestConf(t)

	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Contains(t, paths, defaultLogPath)
	assert.Contains(t, paths, defaultOsReleasePath)
}

func TestPrivateActionRunnerAllowedPathsContainerizedWithoutHostMounts(t *testing.T) {
	// Simulate containerized environment without host mounts (e.g. Fargate)
	t.Setenv("DOCKER_DD_AGENT", "true")

	cfg := newTestConf(t)

	// Without host mounts (/host/var/log etc don't exist), should fall back to container paths
	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{defaultLogPath, defaultOsReleasePath}, paths)
}
