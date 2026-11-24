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
// threshold and runs the scan, so getStats().UnscheduledChecks > 0 — simulating
// checks that have been stranded long enough to be considered an outage (rather
// than the transient startup/failover window).
func markDanglingChecksStuck(d *dispatcher) {
	d.store.Lock()
	for _, c := range d.store.danglingConfigs {
		c.timeCreated = time.Now().Add(-24 * time.Hour)
	}
	d.store.Unlock()
	d.scanUnscheduledChecks()
}

func TestCheckCLCRunnersHealth_ReportsWhenChecksStuckAndNoWorkers(t *testing.T) {
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
// freshly-scheduled checks with no workers yet (the startup / leader-failover /
// runner-rollout window) must NOT raise an issue until they are stuck past the
// unscheduled-check threshold.
func TestCheckCLCRunnersHealth_NoIssueDuringStartupWindow(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	// Just scheduled, no workers yet, but not stuck long enough: transient, not an outage.
	d.Schedule([]integration.Config{generateIntegration("cluster-check")})

	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID),
		"must not report during the transient window before checks are stuck")
}

func TestCheckCLCRunnersHealth_NoIssueWhenCLCRunnersPresent(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	d.processNodeStatus("runner1", "10.0.0.10", types.NodeStatus{LastChange: 1, NodeType: types.NodeTypeCLCRunner})
	markDanglingChecksStuck(d)

	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
}

func TestCheckCLCRunnersHealth_NoIssueWhenNodeAgentsPresent(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	// Node agents can run the cluster checks, so no metrics are lost.
	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	d.processNodeStatus("agent1", "10.0.0.20", types.NodeStatus{LastChange: 1, NodeType: types.NodeTypeNodeAgent})
	markDanglingChecksStuck(d)

	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
}

func TestCheckCLCRunnersHealth_NoIssueWhenNoChecks(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	// No workers, but also nothing to dispatch: not an incident.
	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
}

func TestCheckCLCRunnersHealth_ResolvesWhenRunnersReturn(t *testing.T) {
	d, hp := newHealthTestDispatcher(t)

	d.Schedule([]integration.Config{generateIntegration("cluster-check")})
	markDanglingChecksStuck(d)
	d.checkCLCRunnersHealth()
	require.NotNil(t, hp.GetIssue(missingclcrunners.IssueID))

	// A runner comes back: the issue must be resolved.
	d.processNodeStatus("runner1", "10.0.0.10", types.NodeStatus{LastChange: 1, NodeType: types.NodeTypeCLCRunner})
	d.checkCLCRunnersHealth()

	assert.Nil(t, hp.GetIssue(missingclcrunners.IssueID))
	assert.Contains(t, hp.ResolvedIDs(), missingclcrunners.IssueID)
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
