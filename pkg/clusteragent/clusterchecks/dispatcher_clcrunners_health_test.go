// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && test

package clusterchecks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/missingclcrunners"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

// newHealthTestDispatcher returns a dispatcher wired to a mock health platform store.
func newHealthTestDispatcher(t *testing.T) (*dispatcher, *healthplatformmock.Mock) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	hp := healthplatformmock.New(t)
	return newDispatcher(fakeTagger, hp), hp
}

// markDanglingChecksStuck ages all dangling configs past the unscheduled-check
// threshold and runs the scan, so they are considered stranded rather than in
// the transient startup/failover window.
func markDanglingChecksStuck(d *dispatcher) {
	d.store.Lock()
	for _, c := range d.store.danglingConfigs {
		c.timeCreated = time.Now().Add(-24 * time.Hour)
	}
	d.store.Unlock()
	d.scanUnscheduledChecks()
}

func TestCheckCLCRunnersHealth_ReportsWhenChecksStrandedNoWorkers(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	// A cluster check with no node of any type to run it, stuck past the threshold.
	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	markDanglingChecksStuck(d)

	d.checkCLCRunnersHealth()

	issue := hp.GetIssue(missingclcrunners.IssueID)
	require.NotNil(t, issue, "expected the missing cluster check runners issue to be reported")
	assert.Equal(t, missingclcrunners.IssueName, issue.IssueName)
	assert.Equal(t, missingclcrunners.IssueType, issue.IssueType)
	assert.Equal(t, "1", issue.Extra.Fields["pending_checks"].GetStringValue())
}

// TestCheckCLCRunnersHealth_NoIssueDuringStartupWindow is the debounce guarantee:
// freshly-scheduled checks with no workers yet must NOT raise an issue until they
// are stuck past the unscheduled-check threshold.
func TestCheckCLCRunnersHealth_NoIssueDuringStartupWindow(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	d.Schedule([]integration.Config{generateIntegration("cluster-check")})

	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID),
		"must not report during the transient window before checks are stuck")
}

func TestCheckCLCRunnersHealth_NoIssueWhenEligibleWorkerPresent(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	// A general worker that can run the (ungrouped) check: not stranded.
	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	d.processNodeStatus("runner1", "10.0.0.10", types.NodeStatus{LastChange: 1, NodeType: types.NodeTypeCLCRunner})
	markDanglingChecksStuck(d)

	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
}

// TestCheckCLCRunnersHealth_ReportsWhenRunnerGroupHasNoRunner covers the
// eligibility rule (finding 1): a check claimed by a runner group is stranded
// when that group has no live runner, even though other workers are present.
func TestCheckCLCRunnersHealth_ReportsWhenRunnerGroupHasNoRunner(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)
	d.checkGroup = map[string]string{"grouped-check": "groupA"}
	d.runnerGroups = map[string]struct{}{"groupA": {}}

	// The grouped check can only run in groupA, but the only worker is general.
	d.Schedule([]integration.Config{generateIntegration("grouped-check")})
	d.processNodeStatus("general-runner", "10.0.0.10", types.NodeStatus{LastChange: 1, NodeType: types.NodeTypeCLCRunner})
	markDanglingChecksStuck(d)

	d.checkCLCRunnersHealth()

	require.NotNil(t, hp.GetIssue(missingclcrunners.IssueID),
		"a grouped check with no runner in its group is stranded even with other workers present")
}

func TestCheckCLCRunnersHealth_NoIssueWhenNoChecks(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	// No workers, but also nothing to dispatch: not an incident.
	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
}

func TestCheckCLCRunnersHealth_ResolvesWhenWorkerReturns(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	markDanglingChecksStuck(d)
	d.checkCLCRunnersHealth()
	require.NotNil(t, hp.GetIssue(missingclcrunners.IssueID))

	// A worker comes back: the issue must be resolved.
	d.processNodeStatus("runner1", "10.0.0.10", types.NodeStatus{LastChange: 1, NodeType: types.NodeTypeCLCRunner})
	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
	assert.Equal(t, []string{missingclcrunners.IssueID}, hp.ResolvedIDs())
}

// TestCheckCLCRunnersHealth_ResolveIsEdgeTriggered guards finding 2: once the
// issue is resolved, subsequent healthy ticks must NOT call ResolveIssue again
// (which would re-publish a tombstone every tick).
func TestCheckCLCRunnersHealth_ResolveIsEdgeTriggered(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	markDanglingChecksStuck(d)
	d.checkCLCRunnersHealth() // report
	d.processNodeStatus("runner1", "10.0.0.10", types.NodeStatus{LastChange: 1, NodeType: types.NodeTypeCLCRunner})

	// First healthy tick resolves; further healthy ticks must be no-ops.
	d.checkCLCRunnersHealth()
	d.checkCLCRunnersHealth()
	d.checkCLCRunnersHealth()

	assert.Equal(t, []string{missingclcrunners.IssueID}, hp.ResolvedIDs(),
		"ResolveIssue must be called exactly once, not on every healthy tick")
}

// TestCheckCLCRunnersHealth_ClearsRestoredActiveIssueOnRecovery covers finding A:
// after an unclean restart the active issue is restored into the store's
// by-name index (not the payload map), and a fresh leader with no local state
// must still clear it once checks have recovered.
func TestCheckCLCRunnersHealth_ClearsRestoredActiveIssueOnRecovery(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	restored, err := (&missingclcrunners.MissingCLCRunnersIssue{}).BuildIssue(map[string]string{})
	require.NoError(t, err)
	hp := healthplatformmock.New(t, healthplatformmock.WithIssue(restored))
	d := newDispatcher(fakeTagger, hp)

	// No stranded checks: runners recovered while this (now restarted) leader was down.
	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
	assert.Equal(t, []string{missingclcrunners.IssueID}, hp.ResolvedIDs(),
		"a restored active issue must be cleared once, with no local prior report")
}

// TestCheckCLCRunnersHealth_KeepsRestoredIssueWhileChecksPendingUnaged covers the
// restart-mid-outage flap: after an unclean restart the active issue is restored,
// and the dispatcher replays checks with fresh timestamps (no worker yet, not aged
// past the threshold). The health tick must hold — neither resolve the restored
// issue nor re-report — until the checks either recover or age into an outage.
func TestCheckCLCRunnersHealth_KeepsRestoredIssueWhileChecksPendingUnaged(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	restored, err := (&missingclcrunners.MissingCLCRunnersIssue{}).BuildIssue(map[string]string{})
	require.NoError(t, err)
	hp := healthplatformmock.New(t, healthplatformmock.WithIssue(restored))
	d := newDispatcher(fakeTagger, hp)

	// Replayed check: no worker, but fresh (not marked unscheduled) — the outage
	// is still ongoing but the dispatcher has not re-observed it as stuck yet.
	d.Schedule([]integration.Config{generateIntegration("cluster-check")})

	d.checkCLCRunnersHealth()

	assert.NotNil(t, hp.GetIssue(missingclcrunners.IssueID),
		"restored issue must stay active while checks are pending with no worker")
	assert.Empty(t, hp.ResolvedIDs(), "must not resolve during the replay window")
}

// TestCheckCLCRunnersHealth_NoResolveWhenNeverReported ensures a healthy agent
// that never reported does not emit spurious resolutions.
func TestCheckCLCRunnersHealth_NoResolveWhenNeverReported(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	d.checkCLCRunnersHealth()
	d.checkCLCRunnersHealth()

	assert.Empty(t, hp.ResolvedIDs())
}

func TestCheckCLCRunnersHealth_NilHealthPlatformIsNoop(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	d := newDispatcher(fakeTagger, nil)
	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	markDanglingChecksStuck(d)

	// Must not panic when the health platform is unavailable.
	assert.NotPanics(t, func() {
		d.checkCLCRunnersHealth()
		d.resolveMissingCLCRunnersIssue()
	})
}
