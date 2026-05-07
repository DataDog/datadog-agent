// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	PAREnabled = "private_action_runner.enabled"
	PARLogFile = "private_action_runner.log_file"

	// Identity / enrollment configuration
	PARSelfEnroll           = "private_action_runner.self_enroll"
	PARApiKeyOnlyEnrollment = "private_action_runner.api_key_only_enrollment"
	PARIdentityFilePath     = "private_action_runner.identity_file_path"
	PARIdentityUseK8sSecret = "private_action_runner.identity_use_k8s_secret"
	PARIdentitySecretName   = "private_action_runner.identity_secret_name"
	PARPrivateKey           = "private_action_runner.private_key"
	PARUrn                  = "private_action_runner.urn"

	// General config
	PARTaskConcurrency       = "private_action_runner.task_concurrency"
	PARTaskTimeoutSeconds    = "private_action_runner.task_timeout_seconds"
	PARActionsAllowlist      = "private_action_runner.actions_allowlist"
	PARDefaultActionsEnabled = "private_action_runner.default_actions_enabled"

	// HTTP Action related
	PARHttpTimeoutSeconds    = "private_action_runner.http_timeout_seconds"
	PARHttpAllowlist         = "private_action_runner.http_allowlist"
	PARHttpAllowImdsEndpoint = "private_action_runner.http_allow_imds_endpoint"

	// Restricted Shell
	PARRestrictedShellAllowedPaths     = "private_action_runner.restricted_shell.allowed_paths"
	PARRestrictedShellAllowedCommands  = "private_action_runner.restricted_shell.allowed_commands"
	RShellCommandNamespacePrefix       = "rshell:"
	RShellCommandAllowAllWildcard      = RShellCommandNamespacePrefix + "*"
	RShellPathAllowAll                 = "/"
	RShellPathAllowMapContainerizedKey = "containerized"
	RShellPathAllowMapDefaultKey       = "default"

	// Meant for internal usage
	PAROpmsExtraHeaders = "private_action_runner.opms_extra_headers"
)

// setupPrivateActionRunner registers all configuration keys for the private action runner
func setupPrivateActionRunner(config pkgconfigmodel.Setup) {
	// Enable/disable private action runner
	config.BindEnvAndSetDefault(PAREnabled, false)

	// Log file
	config.BindEnvAndSetDefault(PARLogFile, DefaultPrivateActionRunnerLogFile)

	// Identity / enrollment configuration
	config.BindEnvAndSetDefault(PARSelfEnroll, true)
	config.BindEnvAndSetDefault(PARApiKeyOnlyEnrollment, false)
	config.BindEnvAndSetDefault(PARIdentityFilePath, "")
	config.BindEnvAndSetDefault(PARIdentityUseK8sSecret, true)
	config.BindEnvAndSetDefault(PARIdentitySecretName, "private-action-runner-identity")
	config.BindEnvAndSetDefault(PARPrivateKey, "")
	config.BindEnvAndSetDefault(PARUrn, "")

	// General config
	config.BindEnvAndSetDefault(PARTaskConcurrency, 5)
	config.BindEnvAndSetDefault(PARTaskTimeoutSeconds, 60)
	config.BindEnvAndSetDefault(PARActionsAllowlist, []string{})
	config.BindEnvAndSetDefault(PARDefaultActionsEnabled, true)
	config.ParseEnvSplitComma(PARActionsAllowlist)

	// HTTP action
	config.BindEnvAndSetDefault(PARHttpTimeoutSeconds, 30)
	config.BindEnvAndSetDefault(PARHttpAllowlist, []string{})
	config.ParseEnvSplitComma(PARHttpAllowlist)
	config.BindEnvAndSetDefault(PARHttpAllowImdsEndpoint, false)

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
	config.BindEnvAndSetDefault(PARRestrictedShellAllowedPaths, []string{RShellPathAllowAll})
	pkgconfighelper.ParseEnvJSONOrComma(PARRestrictedShellAllowedPaths, config)

	config.BindEnvAndSetDefault(PARRestrictedShellAllowedCommands, []string{RShellCommandAllowAllWildcard})
	pkgconfighelper.ParseEnvJSONOrComma(PARRestrictedShellAllowedCommands, config)

	config.BindEnvAndSetDefault(PAROpmsExtraHeaders, map[string]string{})
}
