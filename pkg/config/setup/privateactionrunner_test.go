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
	t.Setenv("DOCKER_DD_AGENT", "")
	os.Unsetenv("DOCKER_DD_AGENT")

	cfg := newTestConf(t)

	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{defaultLogPath, defaultOsReleasePath}, paths)
}

func TestPrivateActionRunnerAllowedPathsContainerizedWithHostMounts(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	// Point prefix at a temp dir with simulated host mounts
	tmpDir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "var", "log"), 0755))
	assert.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "etc"), 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "etc", "os-release"), []byte(""), 0644))

	original := containerizedPathPrefix
	containerizedPathPrefix = tmpDir
	t.Cleanup(func() { containerizedPathPrefix = original })

	cfg := newTestConf(t)

	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{
		filepath.Join(tmpDir, "var", "log"),
		filepath.Join(tmpDir, "etc", "os-release"),
	}, paths)
}

func TestPrivateActionRunnerAllowedPathsContainerizedWithoutHostMounts(t *testing.T) {
	t.Setenv("DOCKER_DD_AGENT", "true")

	// Point prefix at an empty temp dir — no host mounts
	tmpDir := t.TempDir()

	original := containerizedPathPrefix
	containerizedPathPrefix = tmpDir
	t.Cleanup(func() { containerizedPathPrefix = original })

	cfg := newTestConf(t)

	// Without host mounts, should fall back to container paths
	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{defaultLogPath, defaultOsReleasePath}, paths)
}
