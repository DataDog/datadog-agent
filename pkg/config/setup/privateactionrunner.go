// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	"path"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/env"
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
	PARRestrictedShellAllowedPaths = "private_action_runner.restricted_shell_allowed_paths"
)

const (
	// Default allowed paths for restricted shell
	defaultLogPath        = "/var/log"
	defaultWindowsLogPath = `C:\ProgramData\Datadog\logs`

	containerizedPathPrefix = "/host"
)

// parPathExists is the function used to check path existence. It defaults to
// pathExists and can be overridden in tests.
var parPathExists = pathExists

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
	config.ParseEnvAsStringSlice(PARActionsAllowlist, func(s string) []string {
		return strings.Split(s, ",")
	})

	// HTTP action
	config.BindEnvAndSetDefault(PARHttpTimeoutSeconds, 30)
	config.BindEnvAndSetDefault(PARHttpAllowlist, []string{})
	config.ParseEnvAsStringSlice(PARHttpAllowlist, func(s string) []string {
		return strings.Split(s, ",")
	})
	config.BindEnvAndSetDefault(PARHttpAllowImdsEndpoint, false)

	var defaultPaths []string
	if runtime.GOOS == "windows" {
		defaultPaths = []string{defaultWindowsLogPath}
	} else {
		defaultPaths = []string{defaultLogPath}
		if env.IsContainerized() {
			for i, v := range defaultPaths {
				hostPath := path.Join(containerizedPathPrefix, v)
				defaultPaths[i] = hostPath
			}
		}
	}
	config.BindEnvAndSetDefault(PARRestrictedShellAllowedPaths, defaultPaths)
	config.ParseEnvAsStringSlice(PARRestrictedShellAllowedPaths, func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(s, ",")
	})
}
