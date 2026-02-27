// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"slices"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// IsCheckAllowed returns true if the check is allowed.
// When not in basic mode, all checks are allowed (returns true).
// When in basic mode, only checks in the allowed list or starting with "custom_" are permitted.
// Note: Legacy key (allowed_additional_checks) is aliased to mode-specific
// keys in config.go via applyInfrastructureModeOverrides.
func IsCheckAllowed(checkName string, cfg pkgconfigmodel.Reader) bool {
	if !cfg.GetBool("integration.enabled") {
		return false
	}

	infraMode := cfg.GetString("infrastructure_mode")

	// Check excluded list
	if slices.Contains(cfg.GetStringSlice("integration.excluded"), checkName) {
		return false
	}

	// Allow all custom checks
	if strings.HasPrefix(checkName, "custom_") {
		return true
	}

	// If allowed checks is empty, all checks are allowed
	if allowedChecks := cfg.GetStringSlice("integration." + infraMode + ".allowed"); len(allowedChecks) == 0 || slices.Contains(allowedChecks, checkName) {
		return true
	}

	// Check additional list
	return slices.Contains(cfg.GetStringSlice("integration.additional"), checkName)
}
