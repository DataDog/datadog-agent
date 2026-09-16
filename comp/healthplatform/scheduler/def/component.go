// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package scheduler defines the interface for the health platform scheduler
// (the periodic runner of built-in health checks).
package scheduler

import (
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// team: fleet-remediation

// Component is the health-platform scheduler component.
type Component interface {
	// Schedule registers fn to run at the given interval. The scheduler
	// maintains per-registration state: after each tick it resolves any
	// IssueIds that disappeared from the previous run. If interval is zero
	// or negative, the scheduler's default interval is used.
	//
	// initialIssueIDs pre-populates the per-check lastIssueIDs set; pass the
	// IDs of any issues that were active in the store before this call so that
	// they are resolved on the first tick if the check no longer reports them.
	// Callers should obtain these IDs via store.GetActiveIssueIDsByIssueType.
	Schedule(source string, fn runnerdef.HealthCheckFunc, interval time.Duration, initialIssueIDs []string) error
}
