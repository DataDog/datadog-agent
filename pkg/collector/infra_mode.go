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
	// When not in basic mode, all checks are allowed
	if cfg.GetString("infrastructure_mode") != "basic" {
		return true
	}

	// Allow all custom checks
	if strings.HasPrefix(checkName, "custom_") {
		return true
	}

	// Check if it's in the allowed checks (default + additional)
	return slices.Contains(append(cfg.GetStringSlice("allowed_checks"), cfg.GetStringSlice("allowed_additional_checks")...), checkName)
}
