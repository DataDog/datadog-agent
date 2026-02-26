// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	clocktesting "k8s.io/utils/clock/testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/spot"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func runTestScheduler(t *testing.T, cluster *fakeCluster) (*spot.Scheduler, *clocktesting.FakeClock) {
	config := spot.Config{
		Percentage:          50,
		MinOnDemandReplicas: 1,
		ScheduleTimeout:     1 * time.Minute,
		DisabledInterval:    2 * time.Minute,
	}
	rollout := spot.RolloutFunc(func(context.Context, spot.OwnerKey, time.Time) (bool, error) {
		return true, nil
	})
	isLeader := func() bool {
		return true
	}

	clk := clocktesting.NewFakeClock(time.Now())

	scheduler := spot.NewSchedulerForTest(config, clk, cluster.WLM(), rollout, isLeader)
	go scheduler.Run(t.Context())
	<-scheduler.WaitSubscribed()

	cluster.AddAdmissionHook(scheduler.ApplyRecommendations)

	return scheduler, clk
}

func updateDeployment(cluster *fakeCluster, namespace, name string, replicas int, annotations map[string]string, currentReplicaSet string) string {
	// A new ReplicaSet created
	newReplicaSet := replicaSetName(name)
	for _, pod := range newPods(kubernetes.ReplicaSetKind, namespace, newReplicaSet, replicas, annotations) {
		cluster.CreatePod(pod)
	}
	// Old ReplicaSet is scaled down
	if currentReplicaSet != "" {
		cluster.DeleteOwnerPods(kubernetes.ReplicaSetKind, namespace, currentReplicaSet)
	}
	return newReplicaSet
}

// spotAnnotations returns annotations to enable spot scheduling.
// Pass negative value to omit specific annotation.
func spotAnnotations(percentage, minOnDemand int) map[string]string {
	annotations := map[string]string{spot.SpotEnabledAnnotation: "true"}
	if percentage >= 0 {
		annotations[spot.SpotPercentageAnnotation] = strconv.Itoa(percentage)
	}
	if minOnDemand >= 0 {
		annotations[spot.SpotMinOnDemandReplicasAnnotation] = strconv.Itoa(minOnDemand)
	}
	return annotations
}

func TestScenarios(t *testing.T) {
	t.Run("Single pod scheduled to on-demand node due to min on-demand replicas constraint", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t, cluster)

		// When
		rs := updateDeployment(cluster, "default", "nginx", 1, spotAnnotations(100, 2), "")

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", 1))
	})

	t.Run("Rolling update preserves ratio", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		const replicas = 10
		rs1 := updateDeployment(cluster, "default", "nginx", replicas, nil, "")

		runTestScheduler(t, cluster)

		// When
		rs2 := updateDeployment(cluster, "default", "nginx", replicas, spotAnnotations(60, 2), rs1)

		// Then: 60% spot / 40% on-demand
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs2, expectRunning("spot", 6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs2, expectRunning("on-demand", 4))
	})

	t.Run("Scale-up preserves ratio", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t, cluster)

		// When: initial deployment with 5 replicas at 60%
		annotations := spotAnnotations(60, 2)
		rs := updateDeployment(cluster, "default", "nginx", 5, annotations, "")

		// Then: 2 on-demand (minOnDemand=2), 3 spot
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", 2))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("spot", 3))

		// When: scale up to 10 replicas
		rs = updateDeployment(cluster, "default", "nginx", 10, annotations, rs)

		// Then: 4 on-demand, 6 spot (60% of 10, minOnDemand=2)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", 4))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("spot", 6))
	})

	t.Run("Changing spot percentage updates ratio", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t, cluster)

		// When
		const replicas = 10
		const minOnDemand = 2
		rsName := updateDeployment(cluster, "default", "nginx", replicas, spotAnnotations(0, minOnDemand), "")

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rsName, expectRunning("on-demand", replicas))

		// When
		steps := []struct {
			percentage int
			spot       int
		}{
			{10, 1},
			{20, 2},
			{30, 3},
			{40, 4},
			{50, 5},
			{60, 6},
			{70, 7},
			{80, 8},
			{90, 8},  // clamped by minOnDemand=2
			{100, 8}, // clamped by minOnDemand=2
		}

		for _, step := range steps {
			// When
			rsName = updateDeployment(cluster, "default", "nginx", replicas, spotAnnotations(step.percentage, minOnDemand), rsName)

			// Then
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rsName, expectRunning("on-demand", replicas-step.spot))
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rsName, expectRunning("spot", step.spot))
		}
	})

	t.Run("Fallback to on-demand when spot node unavailable", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		// No spot node: spot-assigned pods stay Pending.

		s, clk := runTestScheduler(t, cluster)

		// When
		annotations := spotAnnotations(60, 2)
		rs1 := updateDeployment(cluster, "default", "nginx", 10, annotations, "")

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs1, expectRunning("on-demand", 4))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs1, expectPending(6))

		// When
		// Schedule timeout
		clk.Step(s.Config().ScheduleTimeout + 1*time.Nanosecond)

		// Then
		require.Eventually(t, func() bool {
			_, disabled := s.IsSpotSchedulingDisabled()
			return disabled
		}, 1*time.Second, 50*time.Millisecond)

		// When
		rs2 := updateDeployment(cluster, "default", "nginx", 10, annotations, rs1)

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs2, expectRunning("on-demand", 10))

		// When
		cluster.AddSpotNode("spot")
		// Advance past disabled interval to re-enable spot scheduling
		clk.Step(s.Config().DisabledInterval + time.Second)
		require.Eventually(t, func() bool {
			_, disabled := s.IsSpotSchedulingDisabled()
			return !disabled
		}, 1*time.Second, 50*time.Millisecond)

		rs3 := updateDeployment(cluster, "default", "nginx", 10, annotations, rs2)

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs3, expectRunning("spot", 6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs3, expectRunning("on-demand", 4))
	})
}

func expectRunning(node string, count int) func([]*workloadmeta.KubernetesPod) bool {
	return func(pods []*workloadmeta.KubernetesPod) bool {
		actual := 0
		for _, p := range pods {
			if p.Phase == string(corev1.PodRunning) && p.NodeName == node {
				actual++
			}
		}
		return actual == count
	}
}

func expectPending(count int) func([]*workloadmeta.KubernetesPod) bool {
	return func(pods []*workloadmeta.KubernetesPod) bool {
		actual := 0
		for _, p := range pods {
			if p.Phase == string(corev1.PodPending) {
				actual++
			}
		}
		return actual == count
	}
}
