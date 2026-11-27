// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && kubeapiserver

package healthplatform

import (
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

// team: fleet-remediation

const (
	// slowClusterIDLookup is how long the cluster ID lookup takes in TestSlowClusterIdentityDoesNotBlockReporters,
	// standing in for an unreachable Cluster Agent
	slowClusterIDLookup = 30 * time.Second
	testClusterID       = "11111111-1111-1111-1111-111111111111"
)

// TestSlowClusterIdentityDoesNotBlockReporters reproduces the other half of the incident this store follows up on:
// ReportIssue used to look up this agent's cluster identity before returning, so a slow Cluster Agent stalled the
// caller. On Kubernetes, the cluster ID lookup here takes slowClusterIDLookup. A reporter, standing in for
// Autodiscovery, reports an issue every second while it is stuck: each call must return without waiting, the health
// payloads must keep reaching the intake, and once the lookup finishes they carry the cluster ID, also for the issues
// reported before.
func TestSlowClusterIdentityDoesNotBlockReporters(t *testing.T) {
	initProcessGlobals()
	synctest.Test(t, func(t *testing.T) {
		fiClient, opts := storeTestEnv(t, t.TempDir())
		// after storeTestEnv, which clears the inherited DD_* variables
		env.SetFeatures(t, env.Kubernetes)
		t.Setenv("DD_POD_NAME", "dd-agent-abc12")
		var lookups atomic.Int32
		selfident.SetClusterIDLookup(t, func() (string, error) {
			lookups.Add(1)
			time.Sleep(slowClusterIDLookup)
			return testClusterID, nil
		})
		deps := fxutil.Test[struct {
			fx.In
			Store storedef.Component
			WMeta workloadmetamock.Mock
		}](t, opts)
		store := deps.Store
		// This agent's pod, owned by a DaemonSet, so the deployment ID resolves at once: only the cluster ID is slow.
		deps.WMeta.Set(&workloadmeta.KubernetesPod{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "self-pod-uid"},
			EntityMeta: workloadmeta.EntityMeta{Name: "dd-agent-abc12", Namespace: namespace.GetMyNamespace()},
			Owners:     []workloadmeta.KubernetesPodOwner{{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"}},
		})

		// Report for longer than the lookup, so reports keep coming while it is stuck.
		const reports = 35
		for i := range reports {
			start := time.Now()
			require.NoError(t, store.ReportIssue(storeTestIssue(fmt.Sprintf("store-test:identity:%d", i))))
			// In the bubble, time only moves while every goroutine is blocked: a call waiting on the lookup would
			// advance it.
			require.Zero(t, time.Since(start), "ReportIssue %d waited for the cluster identity", i)
			time.Sleep(time.Second)
		}
		require.Positive(t, lookups.Load(), "the cluster ID lookup never ran")

		waitForReports()
		clusterTag := "cluster_id:" + testClusterID
		for i := range reports {
			id := fmt.Sprintf("store-test:identity:%d", i)
			assert.NotEmpty(t, receivedIssues(t, fiClient, id), "issue %s never reached the intake", id)
		}

		// The first issue was reported before the lookup finished: it was first sent without the cluster ID, and
		// carries it once the lookup finished, without being reported again.
		versions := receivedIssues(t, fiClient, "store-test:identity:0")
		require.NotEmpty(t, versions)
		assert.NotContains(t, versions[0].GetTags(), clusterTag, "the cluster ID can't be known before the lookup finished")
		assert.True(t, slices.ContainsFunc(versions, func(v *healthplatformpayload.Issue) bool {
			return slices.Contains(v.GetTags(), clusterTag)
		}), "the issue never carried the cluster ID once the lookup finished")
	})
}
