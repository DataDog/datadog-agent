// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mockParPathExists(existing map[string]bool) func(string) bool {
	return func(path string) bool {
		return existing[path]
	}
}

func overrideParPathExists(t *testing.T, fn func(string) bool) {
	original := parPathExists
	parPathExists = fn
	t.Cleanup(func() { parPathExists = original })
}

func TestPrivateActionRunnerApiKeyOnlyEnrollmentDefaultFalse(t *testing.T) {
	cfg := newTestConf(t)

	assert.False(t, cfg.GetBool(PARApiKeyOnlyEnrollment))
}

func TestPrivateActionRunnerApiKeyOnlyEnrollmentFromEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_API_KEY_ONLY_ENROLLMENT", "true")

	cfg := newTestConf(t)

	assert.True(t, cfg.GetBool(PARApiKeyOnlyEnrollment))
}

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

	if runtime.GOOS == "windows" {
		assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
	} else {
		assert.Equal(t, []string{defaultLogPath}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
	}
}

func TestPrivateActionRunnerAllowlistDefaultsEmpty(t *testing.T) {
	cfg := newTestConf(t)

	assert.Empty(t, cfg.GetStringSlice(PARActionsAllowlist))
	assert.Empty(t, cfg.GetStringSlice(PARHttpAllowlist))
	if runtime.GOOS == "windows" {
		assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
	} else {
		assert.Equal(t, []string{defaultLogPath}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
	}
}

func TestPrivateActionRunnerAllowedPathsBareMetal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default allowed paths are empty on Windows")
	}
	t.Setenv("DOCKER_DD_AGENT", "")
	os.Unsetenv("DOCKER_DD_AGENT")

	cfg := newTestConf(t)

	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{defaultLogPath}, paths)
}

func TestPrivateActionRunnerAllowedPathsContainerizedWithHostMounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default allowed paths are empty on Windows")
	}
	t.Setenv("DOCKER_DD_AGENT", "true")
	overrideParPathExists(t, mockParPathExists(map[string]bool{
		"/host/var/log": true,
	}))

	cfg := newTestConf(t)

	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{"/host/var/log"}, paths)
}

func TestPrivateActionRunnerAllowedPathsContainerizedWithoutHostMounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("default allowed paths are empty on Windows")
	}
	t.Setenv("DOCKER_DD_AGENT", "true")
	overrideParPathExists(t, mockParPathExists(map[string]bool{}))

	cfg := newTestConf(t)

	// Even without host mounts, containerized paths should use /host prefix
	// (rshell handles missing paths at runtime; config logs a warning)
	paths := cfg.GetStringSlice(PARRestrictedShellAllowedPaths)
	assert.Equal(t, []string{
		path.Join(containerizedPathPrefix, defaultLogPath),
	}, paths)
}

func TestPrivateActionRunnerAllowedPathsWindowsDefaultEmpty(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only: default allowed paths should be empty")
	}

	cfg := newTestConf(t)

	assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}
