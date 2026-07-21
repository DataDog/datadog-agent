// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

const (
	PAREnabled = "private_action_runner.enabled"
	PARLogFile = "private_action_runner.log_file"

	// Identity / enrollment configuration
	PARSelfEnroll             = "private_action_runner.self_enroll"
	PARApiKeyOnlyEnrollment   = "private_action_runner.api_key_only_enrollment"
	PARIdentityFilePath       = "private_action_runner.identity_file_path"
	PARIdentityUseK8sSecret   = "private_action_runner.identity_use_k8s_secret"
	PARIdentitySecretName     = "private_action_runner.identity_secret_name"
	PARPrivateKey             = "private_action_runner.private_key"
	PARUrn                    = "private_action_runner.urn"
	PARSkipConnectionCreation = "private_action_runner.skip_connection_creation"

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
	PARRestrictedShellAllowedPaths      = "private_action_runner.restricted_shell.allowed_paths"
	PARRestrictedShellAllowedCommands   = "private_action_runner.restricted_shell.allowed_commands"
	PARRestrictedShellPrivilegedEnabled = "private_action_runner.restricted_shell.privileged.enabled"
	PARRestrictedShellPrivilegedSocket  = "private_action_runner.restricted_shell.privileged.socket"
	RShellCommandNamespacePrefix        = "rshell:"
	RShellCommandAllowAllWildcard       = RShellCommandNamespacePrefix + "*"
	RShellPathAllowAll                  = "/"
	RShellPathAllowMapContainerizedKey  = "containerized"
	RShellPathAllowMapDefaultKey        = "default"
	RShellPrivilegedSocketDefault       = "/run/datadog/rshell-privileged.sock"

	// Meant for internal usage
	PAROpmsExtraHeaders = "private_action_runner.opms_extra_headers"
)
