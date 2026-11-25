// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

// TestPods checks the pods collector, which only lists unassigned pods, and the terminated pods collector, which
// reports the deletions of every pod. The cluster also has an object of every other resource.
func TestPods(t *testing.T) {
	t.Run("assigned_pod_is_sent_when_deleted", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCollectors("pods", "terminated-pods"))
		pod := suite.AssignedPod("assigned")
		env.Add(t, pod)

		suite.AssertObjects(t, []string{"live v1_pods"}, env.Pop(t, "live v1_pods"))
		env.Delete(t, pod)
		suite.AssertObjects(t, []string{"deleted assigned"}, env.Pop(t, "deleted assigned"))
	})

	// A pending pod is sent until a node runs it: the API server reports a scheduled pod as deleted to the watch of
	// the unassigned pods. The informer handles events in order, so the update of another pending pod shows that it
	// handled the scheduling.
	t.Run("pending_pod_is_scheduled", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCollectors("pods", "terminated-pods"), suite.WithExtraObjects(suite.PendingPod("other")))

		suite.AssertObjects(t, []string{"live other", "live v1_pods"}, env.Pop(t, "live other", "live v1_pods"))
		scheduled := suite.Updated(t, env.Object(t, "v1_pods")).(*corev1.Pod)
		scheduled.Spec.NodeName = "node"
		env.Update(t, scheduled)
		env.Update(t, suite.Updated(t, env.Object(t, "other")))
		suite.AssertObjects(t, []string{"live other"}, env.Pop(t, "live other"))
		// An informer that still held the scheduled pod would send it again once the cache expires.
		env.ExpireResourceCache()
		suite.AssertObjects(t, []string{"live other"}, env.Pop(t, "live other"))
		env.Delete(t, scheduled)
		suite.AssertObjects(t, []string{"deleted v1_pods"}, env.Pop(t, "deleted v1_pods"))
	})
}
