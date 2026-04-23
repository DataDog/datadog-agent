// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestPrivateActionRunnerRestrictedShellAllowedPathsUnsetByDefault(t *testing.T) {
	cfg := newTestConf(t)

	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedPaths))
	assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerRestrictedShellAllowedPathsFromEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_PATHS", "/var/log,/tmp")

	cfg := newTestConf(t)

	assert.True(t, cfg.IsConfigured(PARRestrictedShellAllowedPaths))
	assert.Equal(t, []string{"/var/log", "/tmp"}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerRestrictedShellAllowedPathsEmptyEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_PATHS", "")

	cfg := newTestConf(t)

	// Empty env is treated the same as unset so a stray empty var does not
	// accidentally block every filesystem access.
	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedPaths))
	assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerAllowlistDefaultsEmpty(t *testing.T) {
	cfg := newTestConf(t)

	assert.Empty(t, cfg.GetStringSlice(PARActionsAllowlist))
	assert.Empty(t, cfg.GetStringSlice(PARHttpAllowlist))
	assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerRestrictedShellAllowedCommandsUnsetByDefault(t *testing.T) {
	cfg := newTestConf(t)

	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedCommands))
	assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedCommands))
}

func TestPrivateActionRunnerRestrictedShellAllowedCommandsFromEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_COMMANDS", "cat,ls,grep")

	cfg := newTestConf(t)

	assert.True(t, cfg.IsConfigured(PARRestrictedShellAllowedCommands))
	assert.Equal(t, []string{"cat", "ls", "grep"}, cfg.GetStringSlice(PARRestrictedShellAllowedCommands))
}

func TestPrivateActionRunnerRestrictedShellAllowedCommandsEmptyEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_COMMANDS", "")

	cfg := newTestConf(t)

	// Empty env is treated the same as unset so a stray empty var does not
	// accidentally block every command on the agent.
	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedCommands))
	assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedCommands))
}
