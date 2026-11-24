// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"strconv"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/missingclcrunners"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// checkCLCRunnersHealth reports a health platform issue when the leader has
// cluster checks stuck undispatched with no worker (Cluster Check Runner or node
// agent) to run them, and resolves it once a worker returns.
//
// It gates on UnscheduledChecks (checks dangling past
// cluster_checks.unscheduled_check_threshold) rather than on config count, so
// the transient startup / leader-failover / runner-rollout window — where the
// leader is dispatching but runners have not registered yet — does not raise an
// issue. Operators needing a longer grace raise that threshold.
//
// It is a no-op when health platform reporting is unavailable.
func (d *dispatcher) checkCLCRunnersHealth() {
	if d.healthPlatform == nil {
		return
	}

	clcRunnerCount, nodeAgentCount := d.store.countNodeTypes()
	stats := d.getStats()

	if clcRunnerCount == 0 && nodeAgentCount == 0 && stats.UnscheduledChecks > 0 {
		d.reportMissingCLCRunnersIssue(stats.UnscheduledChecks)
		return
	}

	d.resolveMissingCLCRunnersIssue()
}

// reportMissingCLCRunnersIssue reports (or refreshes) the missing cluster check
// runners health issue, recording how many cluster checks are stuck
// undispatched.
func (d *dispatcher) reportMissingCLCRunnersIssue(stuckChecks int) {
	issue, err := (&missingclcrunners.MissingCLCRunnersIssue{}).BuildIssue(map[string]string{
		"pending_checks": strconv.Itoa(stuckChecks),
	})
	if err != nil {
		log.Warnf("Failed to build missing cluster check runners health issue: %v", err)
		return
	}

	if err := d.healthPlatform.ReportIssue(issue); err != nil {
		log.Warnf("Failed to report missing cluster check runners health issue: %v", err)
	}
}

// resolveMissingCLCRunnersIssue clears the missing cluster check runners health
// issue. It is safe to call when the issue is not currently active.
func (d *dispatcher) resolveMissingCLCRunnersIssue() {
	if d.healthPlatform == nil {
		return
	}
	d.healthPlatform.ResolveIssue(missingclcrunners.IssueID)
}
