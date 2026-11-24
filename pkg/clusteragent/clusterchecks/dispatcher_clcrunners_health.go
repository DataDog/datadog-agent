// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"slices"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/missingclcrunners"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// checkCLCRunnersHealth reports a health platform issue when cluster checks are
// stranded (stuck undispatched with no eligible worker) and resolves it once they
// can be placed again.
//
// Opening and resolving are deliberately asymmetric:
//   - Open when checks have been unplaceable past the unscheduled-check threshold
//     (debounced, so the transient startup/failover/restart window does not alarm).
//   - Resolve only on positive evidence of recovery: every pending check now has
//     an eligible worker (or there are none). In the in-between window — checks
//     still have no worker but have not aged yet — we hold: neither report nor
//     resolve. This is what keeps an inherited/restored issue (e.g. after an
//     unclean restart mid-outage, where replayed checks carry fresh timestamps)
//     active instead of flapping resolved→reopened.
//
// Resolution is edge-triggered off the store's active state rather than a local
// flag: the store re-emits a resolved tombstone on every ResolveIssue call once a
// persisted record exists, so we resolve only when the issue is currently active.
// Reading the active set means a leader that inherited the issue — from a previous
// leadership term or restored from persistence — still clears it once checks
// recover, and leadership handoff never emits a spurious tombstone.
//
// No-op when health platform reporting is unavailable.
func (d *dispatcher) checkCLCRunnersHealth() {
	if d.healthPlatform == nil {
		return
	}

	stranded, pending := d.strandedCheckCounts()

	if stranded > 0 {
		d.reportMissingCLCRunnersIssue(stranded)
		return
	}

	// Checks still have no eligible worker but have not aged past the threshold
	// yet (startup / failover / restart replay): hold — do not report (debounce)
	// and do not resolve (an inherited issue stays active until checks recover).
	if pending > 0 {
		return
	}

	// Recovered: every cluster check can be placed. Clear the issue only if it is
	// currently active, so we don't re-emit a resolved tombstone on every tick.
	activeIDs := d.healthPlatform.GetActiveIssueIDsByIssueName(missingclcrunners.IssueName)
	if slices.Contains(activeIDs, missingclcrunners.IssueID) {
		d.resolveMissingCLCRunnersIssue()
	}
}

// strandedCheckCounts walks the dangling cluster checks that have no eligible
// worker (same eligibility rule as dispatch) and returns:
//   - stranded: those already past the unscheduled-check threshold (an outage)
//   - pending:  all of them regardless of age (pending >= stranded)
//
// The gap between the two is the transient window where a check has no worker
// yet but has not been pending long enough to count as an outage.
//
// A check is dispatchable iff some worker shares its runner group. We build the
// set of groups backed by a worker once, then look each config up, keeping the
// walk O(nodes + configs) under the read lock instead of rescanning nodes per
// config.
func (d *dispatcher) strandedCheckCounts() (stranded, pending int) {
	d.store.RLock()
	defer d.store.RUnlock()

	groupsWithWorkers := make(map[string]struct{}, len(d.store.nodes))
	for _, node := range d.store.nodes {
		groupsWithWorkers[node.group] = struct{}{}
	}

	for _, c := range d.store.danglingConfigs {
		if _, ok := groupsWithWorkers[d.checkGroup[c.config.Name]]; ok {
			continue // has an eligible worker; not stranded
		}
		pending++
		if c.unscheduledCheck {
			stranded++
		}
	}
	return stranded, pending
}

// reportMissingCLCRunnersIssue reports (or refreshes) the missing cluster check
// runners health issue, recording how many cluster checks are stranded.
func (d *dispatcher) reportMissingCLCRunnersIssue(strandedChecks int) {
	issue, err := (&missingclcrunners.MissingCLCRunnersIssue{}).BuildIssue(map[string]string{
		"pending_checks": strconv.Itoa(strandedChecks),
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
