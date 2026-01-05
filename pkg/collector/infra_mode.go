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
func IsCheckAllowed(checkName string, cfg pkgconfigmodel.Reader) bool {
	if !cfg.GetBool("integration.enabled") || slices.Contains(cfg.GetStringSlice("excluded_checks"), checkName) {
		return false
	}

	// Allow all custom checks
	if strings.HasPrefix(checkName, "custom_") {
		return true
	}

	// if allowed checks is empty, all checks are allowed
	if allowedChecks := cfg.GetStringSlice("integration.allowed_checks." + cfg.GetString("infrastructure_mode")); len(allowedChecks) == 0 || slices.Contains(allowedChecks, checkName) {
		return true
	}

	return slices.Contains(cfg.GetStringSlice("allowed_additional_checks"), checkName)
}
