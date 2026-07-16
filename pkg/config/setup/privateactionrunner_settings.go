// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// setupPrivateActionRunner registers all configuration keys for the private action runner
func setupPrivateActionRunner(config pkgconfigmodel.Setup) {
	// Enable/disable private action runner
	config.BindEnvAndSetDefault("private_action_runner.enabled", false)

	// Log file
	config.BindEnvAndSetDefault("private_action_runner.log_file", "${log_path}/private-action-runner.log")

	// Identity / enrollment configuration
	config.BindEnvAndSetDefault("private_action_runner.self_enroll", true)
	config.BindEnvAndSetDefault("private_action_runner.api_key_only_enrollment", false)
	config.BindEnvAndSetDefault("private_action_runner.identity_file_path", "")
	config.BindEnvAndSetDefault("private_action_runner.identity_use_k8s_secret", true)
	config.BindEnvAndSetDefault("private_action_runner.identity_secret_name", "private-action-runner-identity")
	config.BindEnvAndSetDefault("private_action_runner.private_key", "")
	config.BindEnvAndSetDefault("private_action_runner.urn", "")
	config.BindEnvAndSetDefault("private_action_runner.skip_connection_creation", false)

	// General config
	config.BindEnvAndSetDefault("private_action_runner.task_concurrency", 5)
	config.BindEnvAndSetDefault("private_action_runner.task_timeout_seconds", 60)
	config.BindEnvAndSetDefault("private_action_runner.actions_allowlist", []string{})
	config.BindEnvAndSetDefault("private_action_runner.default_actions_enabled", true)
	config.ParseEnvSplitComma("private_action_runner.actions_allowlist")

	// HTTP action
	config.BindEnvAndSetDefault("private_action_runner.http_timeout_seconds", 30)
	config.BindEnvAndSetDefault("private_action_runner.http_allowlist", []string{})
	config.ParseEnvSplitComma("private_action_runner.http_allowlist")
	config.BindEnvAndSetDefault("private_action_runner.http_allow_imds_endpoint", false)

	// Restricted shell allow-lists are opt-in restrictions layered on top of
	// the backend-injected lists. By default, they act as a no-op, allowing
	// everything: the backend is the only filter.
	//
	// To allow none, use an explicit empty list.
	// Env vars support both CSV and JSON-array forms; the JSON form gives
	// env/YAML parity, including the explicit kill-switch via "[]".
	//
	//   - allowed_paths defaults to ["/"].
	//   - allowed_commands defaults to ["rshell:*"]. The wildcard token is
	//     handled as a special case in the operator-side intersection: when
	//     it appears in the operator list, every backend command in the
	//     "rshell:" namespace is admitted.
	config.BindEnvAndSetDefault("private_action_runner.restricted_shell.allowed_paths", []string{RShellPathAllowAll})
	pkgconfighelper.ParseEnvJSONOrComma("private_action_runner.restricted_shell.allowed_paths", config)

	config.BindEnvAndSetDefault("private_action_runner.restricted_shell.allowed_commands", []string{RShellCommandAllowAllWildcard})
	pkgconfighelper.ParseEnvJSONOrComma("private_action_runner.restricted_shell.allowed_commands", config)

	config.BindEnvAndSetDefault("private_action_runner.opms_extra_headers", map[string]string{})
}
