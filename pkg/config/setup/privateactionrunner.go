// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// setupPrivateActionRunner registers all configuration keys for the private action runner
func setupPrivateActionRunner(config pkgconfigmodel.Setup) {
	// Enable/disable private action runner
	config.BindEnvAndSetDefault("privateactionrunner.enabled", false)

	// Log file
	config.BindEnvAndSetDefault("privateactionrunner.log_file", DefaultPrivateActionRunnerLogFile)

	// Self-enrollment configuration
	config.BindEnvAndSetDefault("privateactionrunner.self_enroll", false)
	config.BindEnvAndSetDefault("privateactionrunner.identity_file_path", "")

	// Authentication and identity
	config.BindEnvAndSetDefault("privateactionrunner.private_key", "")
	config.BindEnvAndSetDefault("privateactionrunner.urn", "")

	// Timeout configurations (in seconds)
	config.BindEnvAndSetDefault("privateactionrunner.task_timeout_seconds", 0)
	config.BindEnvAndSetDefault("privateactionrunner.http_timeout_seconds", 30)

	// Security allowlists
	config.BindEnvAndSetDefault("privateactionrunner.actions_allowlist", []string{})
	config.BindEnvAndSetDefault("privateactionrunner.allowlist", "")
	config.BindEnvAndSetDefault("privateactionrunner.allow_imds_endpoint", false)
}
