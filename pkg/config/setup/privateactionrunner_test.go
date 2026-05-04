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

	// Default is the ["/"] sentinel — the operator-side intersection
	// admits every backend-allowed path through containment matching when
	// the operator has not narrowed.
	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedPaths))
	assert.Equal(t, []string{"/"}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
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
	// accidentally block every filesystem access. The default ["/"]
	// sentinel applies, which the operator-side intersection treats as
	// "allow whatever the backend allowed".
	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedPaths))
	assert.Equal(t, []string{"/"}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerAllowlistDefaultsEmpty(t *testing.T) {
	cfg := newTestConf(t)

	assert.Empty(t, cfg.GetStringSlice(PARActionsAllowlist))
	assert.Empty(t, cfg.GetStringSlice(PARHttpAllowlist))
	// PARRestrictedShellAllowedPaths intentionally defaults to ["/"]
	// rather than empty — the sentinel makes the operator-side
	// intersection a no-op when the user hasn't narrowed.
	assert.Equal(t, []string{"/"}, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}

func TestPrivateActionRunnerRestrictedShellAllowedCommandsUnsetByDefault(t *testing.T) {
	cfg := newTestConf(t)

	// Default is the ["rshell:*"] wildcard sentinel — the operator-side
	// intersection admits every backend entry in the rshell namespace
	// when the operator has not narrowed.
	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedCommands))
	assert.Equal(t, []string{"rshell:*"}, cfg.GetStringSlice(PARRestrictedShellAllowedCommands))
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
	// accidentally block every command on the agent. The default
	// ["rshell:*"] sentinel applies, which the operator-side intersection
	// treats as "allow whatever the backend allowed".
	assert.False(t, cfg.IsConfigured(PARRestrictedShellAllowedCommands))
	assert.Equal(t, []string{"rshell:*"}, cfg.GetStringSlice(PARRestrictedShellAllowedCommands))
}

// TestPrivateActionRunnerRestrictedShellAllowedPathsJSONArrayEnv covers the
// JSON-array form for env vars, which gives parity with YAML and
// — crucially — lets operators express the kill-switch via "[]".
func TestPrivateActionRunnerRestrictedShellAllowedPathsJSONArrayEnv(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{"JSON array", `["/var/log","/tmp"]`, []string{"/var/log", "/tmp"}},
		{"JSON kill-switch", `[]`, []string{}},
		{"JSON kill-switch with whitespace", `  []  `, []string{}},
		{"single-element JSON array", `["/var/log"]`, []string{"/var/log"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_PATHS", tc.env)

			cfg := newTestConf(t)

			assert.True(t, cfg.IsConfigured(PARRestrictedShellAllowedPaths))
			assert.Equal(t, tc.want, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
		})
	}
}

func TestPrivateActionRunnerRestrictedShellAllowedCommandsJSONArrayEnv(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{"JSON array", `["rshell:cat","rshell:ls"]`, []string{"rshell:cat", "rshell:ls"}},
		{"JSON kill-switch", `[]`, []string{}},
		{"single-element JSON array", `["rshell:cat"]`, []string{"rshell:cat"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_COMMANDS", tc.env)

			cfg := newTestConf(t)

			assert.True(t, cfg.IsConfigured(PARRestrictedShellAllowedCommands))
			assert.Equal(t, tc.want, cfg.GetStringSlice(PARRestrictedShellAllowedCommands))
		})
	}
}

// TestPrivateActionRunnerRestrictedShellAllowedPathsInvalidJSONEnv pins
// that malformed bracketed input is rejected by the parser. The parser
// logs an error and returns nil; the env var still counts as "set" (so
// the registered default is bypassed), and downstream the handler treats
// nil as the kill-switch — a fail-secure outcome consistent with the
// rest of the contract. Operators who hit this should see the error in
// agent logs.
func TestPrivateActionRunnerRestrictedShellAllowedPathsInvalidJSONEnv(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_RESTRICTED_SHELL_ALLOWED_PATHS", `[/var/log,/tmp]`) // missing quotes

	cfg := newTestConf(t)

	assert.Empty(t, cfg.GetStringSlice(PARRestrictedShellAllowedPaths))
}
