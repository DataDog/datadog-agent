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
// cluster checks to dispatch but no worker of any kind (Cluster Check Runner or
// node agent) is reporting to run them. In that situation cluster checks are
// left undispatched and their metrics may be lost. The issue is resolved as
// soon as a worker is available again (or there are no checks to dispatch).
//
// It is a no-op when health platform reporting is unavailable.
func (d *dispatcher) checkCLCRunnersHealth() {
	if d.healthPlatform == nil {
		return
	}

	clcRunnerCount, nodeAgentCount := d.store.countNodeTypes()
	stats := d.getStats()

	if clcRunnerCount == 0 && nodeAgentCount == 0 && stats.TotalConfigs > 0 {
		d.reportMissingCLCRunnersIssue(stats.TotalConfigs)
		return
	}

	d.resolveMissingCLCRunnersIssue()
}

// reportMissingCLCRunnersIssue reports (or refreshes) the missing cluster check
// runners health issue, recording how many cluster checks are stranded.
func (d *dispatcher) reportMissingCLCRunnersIssue(pendingChecks int) {
	issue, err := (&missingclcrunners.MissingCLCRunnersIssue{}).BuildIssue(map[string]string{
		"pending_checks": strconv.Itoa(pendingChecks),
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
