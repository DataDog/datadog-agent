// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
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

	// Read slices once to avoid repeated allocations.
	excludedSlice := cfg.GetStringSlice("integration.excluded")
	allowedSlice := cfg.GetStringSlice("integration." + infraMode + ".allowed")
	additionalSlice := cfg.GetStringSlice("integration.additional")

	// Build O(1) lookup sets.
	excluded := make(map[string]struct{}, len(excludedSlice))
	for _, s := range excludedSlice {
		excluded[s] = struct{}{}
	}

	// Check excluded list
	if _, ok := excluded[checkName]; ok {
		return false
	}

	// Allow all custom checks
	if strings.HasPrefix(checkName, "custom_") {
		return true
	}

	// If allowed checks is empty, all checks are allowed
	if len(allowedSlice) == 0 {
		return true
	}

	allowed := make(map[string]struct{}, len(allowedSlice))
	for _, s := range allowedSlice {
		allowed[s] = struct{}{}
	}
	if _, ok := allowed[checkName]; ok {
		return true
	}

	// Check additional list
	additional := make(map[string]struct{}, len(additionalSlice))
	for _, s := range additionalSlice {
		additional[s] = struct{}{}
	}
	_, ok := additional[checkName]
	return ok
}
