// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package configkeys defines configuration key constants for the private action runner.
package configkeys

const (
	PAREnabled = "private_action_runner.enabled"
	PARLogFile = "private_action_runner.log_file"

	// Identity / enrollment configuration
	PARSelfEnroll       = "private_action_runner.self_enroll"
	PARIdentityFilePath = "private_action_runner.identity_file_path"
	PARPrivateKey       = "private_action_runner.private_key"
	PARUrn              = "private_action_runner.urn"

	// General config
	PARTaskTimeoutSeconds = "private_action_runner.task_timeout_seconds"
	PARActionsAllowlist   = "private_action_runner.actions_allowlist"

	// HTTP Action related
	PARHttpTimeoutSeconds    = "private_action_runner.http_timeout_seconds"
	PARHttpAllowlist         = "private_action_runner.http_allowlist"
	PARHttpAllowImdsEndpoint = "private_action_runner.http_allow_imds_endpoint"
)
