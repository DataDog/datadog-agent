// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profilerec

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Plan and its parts describe what switching to a profile would change on this host.
type (
	Plan           = pkgconfigsetup.LogsPerformanceProfilePlan
	PlanChange     = pkgconfigsetup.LogsPerformanceProfileChange
	PlanBlockedKey = pkgconfigsetup.LogsPerformanceProfileBlockedKey
	PlanSetting    = pkgconfigsetup.LogsPerformanceProfileCurrentSetting
)

// ActiveProfile returns the active profile name, or "" when none is active.
func ActiveProfile(cfg pkgconfigmodel.Reader) string {
	name, _, _, _ := pkgconfigsetup.ResolvedLogsPerformanceProfile(cfg)
	return name
}

// PlanProfile describes what switching to candidate would change; ok is false for an unknown profile.
func PlanProfile(cfg pkgconfigmodel.Reader, candidate string) (Plan, bool) {
	return pkgconfigsetup.PlanLogsPerformanceProfile(cfg, candidate)
}

// Covers reports whether active already applies every setting of candidate.
func Covers(active, candidate string) bool {
	return pkgconfigsetup.LogsPerformanceProfileCovers(active, candidate)
}
