// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	"encoding/json"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	config.ParseEnvAsStringSlice(PARRestrictedShellAllowedPaths, parseAllowListEnvVar(PARRestrictedShellAllowedPaths))

	config.BindEnvAndSetDefault(PARRestrictedShellAllowedCommands, []string{RShellCommandAllowAllWildcard})
	config.ParseEnvAsStringSlice(PARRestrictedShellAllowedCommands, parseAllowListEnvVar(PARRestrictedShellAllowedCommands))
}

// parseAllowListEnvVar parses an rshell allow-list env var. Accepts both
// CSV ("a,b") and JSON-array (["a","b"], []) forms; the JSON form gives
// env/YAML parity, including the explicit kill-switch via "[]".
func parseAllowListEnvVar(key string) func(string) []string {
	return func(s string) []string {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			res := []string{}
			if err := json.Unmarshal([]byte(s), &res); err != nil {
				log.Errorf("%s: invalid JSON env value %q: %v", key, s, err)
				return nil
			}
			return res
		}
		return strings.Split(s, ",")
	}
}
