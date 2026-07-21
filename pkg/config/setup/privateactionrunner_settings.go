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
	config.BindEnvAndSetDefault(PAREnabled, false)

	// Log file
	config.BindEnvAndSetDefault(PARLogFile, "${log_path}/private-action-runner.log")

	// Identity / enrollment configuration
	config.BindEnvAndSetDefault(PARSelfEnroll, true)
	config.BindEnvAndSetDefault(PARApiKeyOnlyEnrollment, false)
	config.BindEnvAndSetDefault(PARIdentityFilePath, "")
	config.BindEnvAndSetDefault(PARIdentityUseK8sSecret, true)
	config.BindEnvAndSetDefault(PARIdentitySecretName, "private-action-runner-identity")
	config.BindEnvAndSetDefault(PARPrivateKey, "")
	config.BindEnvAndSetDefault(PARUrn, "")
	config.BindEnvAndSetDefault(PARSkipConnectionCreation, false)

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

	// Privileged execution is a separate, explicit opt-in. Keeping transport
	// configuration under restricted_shell avoids enabling it with PAR alone.
	config.BindEnvAndSetDefault(PARRestrictedShellPrivilegedEnabled, false)
	config.BindEnvAndSetDefault(PARRestrictedShellPrivilegedSocket, RShellPrivilegedSocketDefault)

	config.BindEnvAndSetDefault(PAROpmsExtraHeaders, map[string]string{})
}
