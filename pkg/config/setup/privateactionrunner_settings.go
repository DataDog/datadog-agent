// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupPrivateActionRunner(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("private_action_runner.enabled", false)

	config.BindEnvAndSetDefault("private_action_runner.log_file", "${log_path}/private-action-runner.log")

	config.BindEnvAndSetDefault("private_action_runner.self_enroll", true)
	config.BindEnvAndSetDefault("private_action_runner.api_key_only_enrollment", false)
	config.BindEnvAndSetDefault("private_action_runner.identity_file_path", "")
	config.BindEnvAndSetDefault("private_action_runner.identity_use_k8s_secret", true)
	config.BindEnvAndSetDefault("private_action_runner.identity_secret_name", "private-action-runner-identity")
	config.BindEnvAndSetDefault("private_action_runner.private_key", "")
	config.BindEnvAndSetDefault("private_action_runner.urn", "")
	config.BindEnvAndSetDefault("private_action_runner.skip_connection_creation", false)

	config.BindEnvAndSetDefault("private_action_runner.task_concurrency", 5)
	config.BindEnvAndSetDefault("private_action_runner.task_timeout_seconds", 60)
	config.BindEnvAndSetDefault("private_action_runner.actions_allowlist", []string{})
	config.ParseEnvSplitComma("private_action_runner.actions_allowlist")
	config.BindEnvAndSetDefault("private_action_runner.default_actions_enabled", true)
	config.BindEnvAndSetDefault("private_action_runner.executor.socket_path", getPlatformDefault(map[string]interface{}{
		"windows": `\\.\pipe\dd-par-executor`,
		"other":   "${run_path}/par-executor.sock",
	}))

	// Enables the Rust control plane and on-demand Go executor on Linux.
	config.BindEnvAndSetDefault("private_action_runner.split_enabled", false)

	// Control-plane settings consumed by par-control.
	config.BindEnvAndSetDefault("private_action_runner.procmgr_socket_path", getPlatformDefault(map[string]interface{}{
		"windows": `\\.\pipe\datadog-procmgrd`,
		"other":   "/var/run/datadog-procmgrd/dd-procmgrd.sock",
	}))
	config.BindEnvAndSetDefault("private_action_runner.executor_process_name", "datadog-agent-action-executor")
	config.BindEnvAndSetDefault("private_action_runner.idle_timeout_seconds", 60)
	config.BindEnvAndSetDefault("private_action_runner.heartbeat_interval_seconds", 20)

	config.BindEnvAndSetDefault("private_action_runner.http_timeout_seconds", 30)
	config.BindEnvAndSetDefault("private_action_runner.http_allowlist", []string{})
	config.ParseEnvSplitComma("private_action_runner.http_allowlist")
	config.BindEnvAndSetDefault("private_action_runner.http_allow_imds_endpoint", false)

	// Restricted shell allow-lists are opt-in restrictions layered on top of
	// the backend-injected lists. By default, they act as a no-op, allowing
	// everything: the backend is the only filter.
	//
	// To allow no paths or commands, use an explicit empty list.
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

	config.BindEnvAndSetDefault("private_action_runner.restricted_shell.allowed_commands", []string{"rshell:*"})
	pkgconfighelper.ParseEnvJSONOrComma("private_action_runner.restricted_shell.allowed_commands", config)

	// Optional local restriction on backend system-service grants. When
	// unset, backend grants pass through; an explicit empty map denies all.
	// The "*" action admits every backend-granted action for an exact service.
	// The environment variable accepts a JSON object only.
	config.BindEnvAndSetDefault("private_action_runner.restricted_shell.allowed_system_services", map[string][]string{})
	config.ParseEnvJSON("private_action_runner.restricted_shell.allowed_system_services", map[string][]string{})

	config.BindEnvAndSetDefault("private_action_runner.opms_extra_headers", map[string]string{})
}
