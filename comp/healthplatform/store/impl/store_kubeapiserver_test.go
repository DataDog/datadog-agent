// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && kubeapiserver

package storeimpl

import (
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

const testSelfPodName = "dd-agent-abc12"

// newSelfPodWorkloadmeta returns a Kubernetes workloadmeta mock that
// selfident looks this agent's pod up in. The pod is absent until
// setSelfDaemonSetPod adds it.
func newSelfPodWorkloadmeta(t *testing.T) workloadmetamock.Mock {
	t.Helper()
	// selfident reads the pod name from DD_POD_NAME directly; there's no exported
	// constant to reference from this package.
	t.Setenv("DD_POD_NAME", testSelfPodName)
	// Resolve cluster_id from the environment rather than a Cluster Agent.
	t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", "11111111-1111-1111-1111-111111111111")
	env.SetFeatures(t, env.Kubernetes)
	return workloadmetaimpl.NewWorkloadMetaMock(workloadmetaimpl.Dependencies{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    logmock.New(t),
		Config: config.NewMock(t),
		Params: workloadmeta.NewParams(),
	})
}

// setSelfDaemonSetPod adds this agent's pod, owned by DaemonSet
// "daemonset-uid-123", to wmeta.
func setSelfDaemonSetPod(wmeta workloadmetamock.Mock) {
	wmeta.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "self-pod-uid",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      testSelfPodName,
			Namespace: namespace.GetMyNamespace(),
		},
		Owners: []workloadmeta.KubernetesPodOwner{
			{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
		},
	})
}

// TestReportIssueEnrichesWithClusterIdentity verifies that the worker stamps
// the deployment_id/cluster_id selfident resolved in the background into
// Extra/Tags, since every other test in this file resolves selfIdent against a
// nil workloadmeta (always empty), which left this enrichment path untested.
func TestReportIssueEnrichesWithClusterIdentity(t *testing.T) {
	wmeta := newSelfPodWorkloadmeta(t)
	setSelfDaemonSetPod(wmeta)

	synctest.Test(t, func(t *testing.T) {
		h := newUnstartedTestStore(t)
		h.selfIdent = selfident.New(wmeta)
		// start kicks off identity resolution; Wait lets it land.
		startTestStore(t, h)
		synctest.Wait()

		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:id", IssueName: "t"}))
		flush(t, h)

		issue := h.GetIssue("t:id")
		require.NotNil(t, issue)
		require.NotNil(t, issue.Extra)
		assert.Equal(t, "daemonset-uid-123", issue.Extra.Fields["deployment_id"].GetStringValue())
		assert.Contains(t, issue.Tags, "deployment_id:daemonset-uid-123")
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", issue.Extra.Fields["cluster_id"].GetStringValue())
		assert.Contains(t, issue.Tags, "cluster_id:11111111-1111-1111-1111-111111111111")
	})
}

// TestReportIssueDoesNotWaitForUnresolvedIdentity verifies that neither callers
// nor the worker wait on a selfident that can't resolve: this agent's pod never
// shows up in workloadmeta, so any wait on DeploymentID lasts its full timeout.
func TestReportIssueDoesNotWaitForUnresolvedIdentity(t *testing.T) {
	wmeta := newSelfPodWorkloadmeta(t)

	h := newUnstartedTestStore(t)
	h.selfIdent = selfident.New(wmeta)
	startTestStore(t, h)

	const reports = 20
	for i := 0; i < reports; i++ {
		require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: fmt.Sprintf("t:%d", i), IssueName: "t"}))
		h.ResolveIssue("unknown")
	}

	// flush times out long before the worker could get through 20 lookups.
	flush(t, h)
	count, issues := h.GetAllIssues()
	assert.Equal(t, reports, count)
	assert.NotContains(t, issues["t:0"].GetExtra().GetFields(), "deployment_id", "no identity to stamp yet")

	// Once selfident resolves, later reports are stamped.
	setSelfDaemonSetPod(wmeta)
	require.Equal(t, "daemonset-uid-123", h.selfIdent.DeploymentID())
	require.NoError(t, h.ReportIssue(&healthplatformpayload.Issue{Id: "t:0", IssueName: "t"}))
	flush(t, h)
	assert.Equal(t, "daemonset-uid-123", h.GetIssue("t:0").GetExtra().GetFields()["deployment_id"].GetStringValue())
}

// TestResourceIdentityDeployment verifies that a node agent running under a
// DaemonSet reports resource_type "deployment" with the DaemonSet UID as
// resource_id.
func TestResourceIdentityDeployment(t *testing.T) {
	wmeta := newSelfPodWorkloadmeta(t)
	setSelfDaemonSetPod(wmeta)

	h := newUnstartedTestStore(t)
	h.selfIdent = selfident.New(wmeta)

	resourceType, resourceID := h.ResourceIdentity("test-host")
	assert.Equal(t, "deployment", resourceType)
	assert.Equal(t, "daemonset-uid-123", resourceID)
}

// TestResourceIdentityClusterAgent verifies that ResourceIdentity dispatches
// to resource_type "cluster" with selfIdent.ClusterID() as resource_id when
// h.agentFlavor is ClusterAgent. It only sets that store-level field, not the
// process-global flavor.GetFlavor() that selfIdent.ClusterID() itself
// dispatches on internally, so ClusterID() here still resolves via the
// node-agent path (which happens to read the same env var below) rather than
// exercising the Cluster-Agent-specific apiserver lookup — that dispatch is
// covered directly by TestClusterID_ClusterAgentFlavorUsesClusterAgentLookup
// and friends in selfident_test.go, which stub both lookups.
func TestResourceIdentityClusterAgent(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)
	t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", "11111111-1111-1111-1111-111111111111")

	h := newUnstartedTestStore(t)
	h.agentFlavor = flavor.ClusterAgent
	h.selfIdent = selfident.New(nil)

	resourceType, resourceID := h.ResourceIdentity("test-host")
	assert.Equal(t, "cluster", resourceType)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", resourceID)
}

// TestResourceIdentityClusterAgentFallsBackToHost verifies that ResourceIdentity
// falls back to resource_type "host" when the Cluster Agent flavor's cluster id
// is unavailable (e.g. the API server hasn't been reachable yet) — resource_id
// must never be silently empty, mirroring the fallback the deployment branch
// already has for an empty DaemonSet UID.
func TestResourceIdentityClusterAgentFallsBackToHost(t *testing.T) {
	h := newUnstartedTestStore(t)
	h.agentFlavor = flavor.ClusterAgent
	// selfident.New outside Kubernetes returns a no-op instance whose
	// ClusterID() is always "", deterministically exercising the fallback
	// without depending on a real (or process-globally cached) cluster id
	// lookup.
	h.selfIdent = selfident.New(nil)

	resourceType, resourceID := h.ResourceIdentity("test-host")
	assert.Equal(t, "host", resourceType)
	assert.Equal(t, "test-host", resourceID)
}
