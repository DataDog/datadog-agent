// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	clocktesting "k8s.io/utils/clock/testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/spot"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func runTestScheduler(ctx context.Context, cluster *fakeCluster) (*spot.Scheduler, *clocktesting.FakeClock) {
	config := spot.Config{
		Percentage:          50,
		MinOnDemandReplicas: 1,
		ScheduleTimeout:     1 * time.Minute,
		DisabledInterval:    2 * time.Minute,
	}

	clk := clocktesting.NewFakeClock(time.Now())

	scheduler := spot.NewTestScheduler(config, clk, cluster.WLM())
	scheduler.Start(ctx)
	<-scheduler.WaitSubscribed()

	cluster.OnPodCreated(scheduler.PodCreated)
	cluster.OnPodDeleted(scheduler.PodDeleted)

	return scheduler, clk
}

func updateDeployment(cluster *fakeCluster, namespace, name string, replicas int, annotations map[string]string, currentReplicaSet string) string {
	// A new ReplicaSet created
	newReplicaSet := replicaSetName(name)
	for range replicas {
		pod := newPod(namespace, kubernetes.ReplicaSetKind, newReplicaSet, annotations)
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

		runTestScheduler(t.Context(), cluster)

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

		runTestScheduler(t.Context(), cluster)

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

		runTestScheduler(t.Context(), cluster)

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

		runTestScheduler(t.Context(), cluster)

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

		s, clk := runTestScheduler(t.Context(), cluster)

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

	t.Run("Pod replacement preserves ratio", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t.Context(), cluster)

		const replicas = 10
		const minOnDemand = 2
		const spotPercentage = 60
		const expectedSpot = 6
		const expectedOnDemand = replicas - expectedSpot
		annotations := spotAnnotations(spotPercentage, minOnDemand)

		rs := updateDeployment(cluster, "default", "nginx", replicas, annotations, "")
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("spot", expectedSpot))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", expectedOnDemand))

		for range 10 {
			// When
			// Delete random pods between 1 and len(pods)-1 and
			// create its replacement simulating the ReplicaSet controller.
			pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
			count := rand.N(len(pods)-1) + 1

			deleted := make(map[string]struct{}, count)
			for _, idx := range rand.Perm(len(pods))[:count] {
				pod := pods[idx]

				cluster.DeletePod(pod)
				deleted[pod.ID] = struct{}{}

				cluster.CreatePod(newPod("default", kubernetes.ReplicaSetKind, rs, annotations))
			}

			// Important: wait until deletion is complete before checking expectations to avoid counting deleted pods.
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectHasNoneOf(deleted))

			// Then
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("spot", expectedSpot))
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", expectedOnDemand))
		}
	})

	t.Run("Pods not eligible for spot are scheduled onto on-demand node", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t.Context(), cluster)

		// When
		rs := updateDeployment(cluster, "default", "nginx", 5, nil, "")

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", 5))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("spot", 0))
	})

	t.Run("Pods not eligible for spot are not tracked", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, _ := runTestScheduler(t.Context(), cluster)

		// When
		rs := updateDeployment(cluster, "default", "nginx", 5, nil, "")

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", 5))
		total, spot := s.TrackedCounts("default", kubernetes.ReplicaSetKind, rs)
		assert.Zero(t, total)
		assert.Zero(t, spot)

		// When
		deleted := make(map[string]struct{}, 5)
		for _, pod := range cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs) {
			cluster.DeletePod(pod)
			deleted[pod.ID] = struct{}{}
		}
		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectHasNoneOf(deleted))
	})

	t.Run("Restarted scheduler tracks existing pods", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		ctx1, stopScheduler := context.WithCancel(t.Context())
		defer stopScheduler()

		runTestScheduler(ctx1, cluster)

		const replicas = 10
		annotations := spotAnnotations(60, 2)
		rs := updateDeployment(cluster, "default", "nginx", replicas, annotations, "")

		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("spot", 6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunning("on-demand", 4))

		// When
		stopScheduler()

		s2, _ := runTestScheduler(t.Context(), cluster)

		// Then
		assert.Eventually(t, func() bool {
			total, spotCount := s2.TrackedCounts("default", kubernetes.ReplicaSetKind, rs)
			return total == replicas && spotCount == 6
		}, 1*time.Second, 10*time.Millisecond)
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

func expectHasNoneOf(ids map[string]struct{}) func([]*workloadmeta.KubernetesPod) bool {
	return func(pods []*workloadmeta.KubernetesPod) bool {
		for _, pod := range pods {
			if _, ok := ids[pod.ID]; ok {
				return false
			}
		}
		return true
	}
}
