// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	parconfigkeys "github.com/DataDog/datadog-agent/pkg/privateactionrunner/configkeys"
)

// setupPrivateActionRunner registers all configuration keys for the private action runner
func setupPrivateActionRunner(config pkgconfigmodel.Setup) {
	// Enable/disable private action runner
	config.BindEnvAndSetDefault(parconfigkeys.PAREnabled, false)

	// Log file
	config.BindEnvAndSetDefault(parconfigkeys.PARLogFile, DefaultPrivateActionRunnerLogFile)

	// Identity / enrollment configuration
	config.BindEnvAndSetDefault(parconfigkeys.PARSelfEnroll, true)
	config.BindEnvAndSetDefault(parconfigkeys.PARIdentityFilePath, "")
	config.BindEnvAndSetDefault(parconfigkeys.PARPrivateKey, "")
	config.BindEnvAndSetDefault(parconfigkeys.PARUrn, "")

	// General config
	config.BindEnvAndSetDefault(parconfigkeys.PARTaskTimeoutSeconds, 60)
	config.BindEnvAndSetDefault(parconfigkeys.PARActionsAllowlist, []string{})

	// HTTP action
	config.BindEnvAndSetDefault(parconfigkeys.PARHttpTimeoutSeconds, 30)
	config.BindEnvAndSetDefault(parconfigkeys.PARHttpAllowlist, []string{})
	config.BindEnvAndSetDefault(parconfigkeys.PARHttpAllowImdsEndpoint, false)
}
