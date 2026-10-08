// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package privateactionrunner contains shared configuration and behavior for the
// private action runner.
package privateactionrunner

const (
	Enabled = "private_action_runner.enabled"
	LogFile = "private_action_runner.log_file"

	// Identity / enrollment configuration
	SelfEnroll             = "private_action_runner.self_enroll"
	APIKeyOnlyEnrollment   = "private_action_runner.api_key_only_enrollment"
	IdentityFilePath       = "private_action_runner.identity_file_path"
	IdentityUseK8sSecret   = "private_action_runner.identity_use_k8s_secret"
	IdentitySecretName     = "private_action_runner.identity_secret_name"
	PrivateKey             = "private_action_runner.private_key"
	URN                    = "private_action_runner.urn"
	SkipConnectionCreation = "private_action_runner.skip_connection_creation"

	// General config
	TaskConcurrency       = "private_action_runner.task_concurrency"
	TaskTimeoutSeconds    = "private_action_runner.task_timeout_seconds"
	IdleTimeoutSeconds    = "private_action_runner.idle_timeout_seconds"
	ActionsAllowlist      = "private_action_runner.actions_allowlist"
	DefaultActionsEnabled = "private_action_runner.default_actions_enabled"
	ExecutorSocketPath    = "private_action_runner.executor.socket_path"
	SplitEnabled          = "private_action_runner.split_enabled"

	// HTTP Action related
	HTTPTimeoutSeconds    = "private_action_runner.http_timeout_seconds"
	HTTPAllowlist         = "private_action_runner.http_allowlist"
	HTTPAllowIMDSEndpoint = "private_action_runner.http_allow_imds_endpoint"

	// Kubernetes action related
	KubernetesAllowedCustomResources = "private_action_runner.kubernetes_allowed_custom_resources"

	// Restricted Shell
	RestrictedShellAllowedPaths             = "private_action_runner.restricted_shell.allowed_paths"
	RestrictedShellAllowedCommands          = "private_action_runner.restricted_shell.allowed_commands"
	RestrictedShellAllowedSystemServices    = "private_action_runner.restricted_shell.allowed_system_services"
	RestrictedShellDisableDetailedTelemetry = "private_action_runner.restricted_shell.disable_detailed_telemetry"
	RestrictedShellPrivilegedEnabled        = "private_action_runner.restricted_shell.privileged.enabled"
	RestrictedShellPrivilegedSocket         = "private_action_runner.restricted_shell.privileged.socket"
	// Unset does not narrow elevation; an explicit empty list denies it.
	RestrictedShellPrivilegedElevatableCommands = "private_action_runner.restricted_shell.privileged.elevatable_commands"
	RShellCommandNamespacePrefix                = "rshell:"
	RShellCommandAllowAllWildcard               = RShellCommandNamespacePrefix + "*"
	RShellPathAllowAll                          = "/"
	RShellPrivilegedSocketDefault               = "/run/datadog/rshell-privileged.sock"

	// Meant for internal usage
	OPMSExtraHeaders = "private_action_runner.opms_extra_headers"
)
