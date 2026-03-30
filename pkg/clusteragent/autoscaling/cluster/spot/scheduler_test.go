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
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/spot"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// epsilon is a small duration added to a clock step to advance past a threshold.
const epsilon = time.Nanosecond

// defaultTestConfig is the spot Config used by all test schedulers.
var defaultTestConfig = spot.Config{
	Percentage:                   50,
	MinOnDemandReplicas:          1,
	ScheduleTimeout:              1 * time.Minute,
	FallbackDuration:             2 * time.Minute,
	RebalanceStabilizationPeriod: 1 * time.Minute,
}

// runTestScheduler creates and starts a Scheduler for testing.
// Returns the scheduler and a fake clock.
func runTestScheduler(ctx context.Context, cluster *fakeCluster) (*spot.Scheduler, *clocktesting.FakeClock) {
	clk := clocktesting.NewFakeClock(time.Now())

	scheduler := spot.NewTestScheduler(defaultTestConfig, clk, cluster.WLM(), cluster.EvictPodByName, cluster.DynamicClient())
	scheduler.Start(ctx)
	scheduler.WaitSynced()

	cluster.OnPodCreated(scheduler.PodCreated)
	cluster.OnPodDeleted(scheduler.PodDeleted)

	return scheduler, clk
}

// spotEnabledLabels returns the labels required to opt a workload into spot scheduling.
func spotEnabledLabels() map[string]string {
	return map[string]string{spot.SpotEnabledLabelKey: spot.SpotEnabledLabelValue}
}

// spotAnnotations returns annotations to register a spot-enabled workload in the store.
// Pass negative value to omit the specific config annotation.
func spotAnnotations(percentage, minOnDemand int) map[string]string {
	annotations := make(map[string]string)
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
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(100, 2), 1)

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningOnDemand(1))
	})

	t.Run("Rolling update preserves ratio", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		const replicas = 10
		runTestScheduler(t.Context(), cluster)
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), replicas)

		// When
		rs2 := d.Rollout(spotEnabledLabels(), spotAnnotations(60, 2), replicas)

		// Then: 60% spot / 40% on-demand
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs2, expectRunningSpot(6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs2, expectRunningOnDemand(4))
	})

	t.Run("Scale-up preserves ratio", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t.Context(), cluster)

		// When: initial deployment with 5 replicas at 60%
		labels := spotEnabledLabels()
		annotations := spotAnnotations(60, 2)
		d := cluster.CreateDeployment("default", "nginx", labels, annotations, 5)

		// Then: 2 on-demand (minOnDemand=2), 3 spot
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningOnDemand(2))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningSpot(3))

		// When: scale up to 10 replicas
		rs := d.Rollout(labels, annotations, 10)

		// Then: 4 on-demand, 6 spot (60% of 10, minOnDemand=2)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
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
		labels := spotEnabledLabels()
		d := cluster.CreateDeployment("default", "nginx", labels, spotAnnotations(0, minOnDemand), replicas)

		// Then
		rsName := d.ReplicaSet()
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rsName, expectRunningOnDemand(replicas))

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
			rsName = d.Rollout(labels, spotAnnotations(step.percentage, minOnDemand), replicas)

			// Then
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rsName, expectRunningOnDemand(replicas-step.spot))
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rsName, expectRunningSpot(step.spot))
		}
	})

	t.Run("Fallback to on-demand when spot node unavailable", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		// No spot node: spot-assigned pods stay Pending.

		s, clk := runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectPending(6))

		// When
		// Schedule timeout
		clk.Step(s.Config().ScheduleTimeout + epsilon)

		// Then
		require.Eventually(t, func() bool {
			return s.IsSpotSchedulingDisabledForOwner("default", kubernetes.ReplicaSetKind, rs)
		}, 1*time.Second, 50*time.Millisecond)

		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
		// Pending pods evicted
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectPending(0))

		// When
		// ReplicaSet recreates pods
		for range 6 {
			cluster.CreatePod(newPod("default", kubernetes.ReplicaSetKind, rs))
		}

		// Then
		// Fallback to on-demand
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(10))

		// When
		cluster.AddSpotNode("new-spot")
		// Advance past disabled interval to re-enable spot scheduling
		clk.Step(s.Config().FallbackDuration + epsilon)
		require.Eventually(t, func() bool {
			return !s.IsSpotSchedulingDisabledForOwner("default", kubernetes.ReplicaSetKind, rs)
		}, 1*time.Second, 50*time.Millisecond)

		// Rebalancing
		for i := range 6 {
			// When
			clk.Step(s.Config().RebalanceStabilizationPeriod + epsilon)

			// Then: excess on-demand pod evicted
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(10-1-i))

			// ReplicaSet recreates pod
			cluster.CreatePod(newPod("default", kubernetes.ReplicaSetKind, rs))
			// Important: wait for it to be Running before next step
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(i+1))
		}

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
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

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(spotPercentage, minOnDemand), replicas)
		rs := d.ReplicaSet()
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(expectedSpot))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(expectedOnDemand))

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

				cluster.CreatePod(newPod("default", kubernetes.ReplicaSetKind, rs))
			}

			// Important: wait until deletion is complete before checking expectations to avoid counting deleted pods.
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectHasNoneOf(deleted))

			// Then
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(expectedSpot))
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(expectedOnDemand))
		}
	})

	t.Run("Rebalancing after scale-down evicts excess on-demand pod", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, clk := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas leaving 2 spot / 3 on-demand — ratio is off
		scaleDown(t, cluster, kubernetes.ReplicaSetKind, "default", rs, 2, 3)

		// When: rebalancing evicts the excess on-demand pod
		clk.Step(s.Config().RebalanceStabilizationPeriod + epsilon)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))

		// ReplicaSet recreates the evicted pod as spot
		cluster.CreatePod(newPod("default", kubernetes.ReplicaSetKind, rs))

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=2 satisfied)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
	})

	t.Run("Rebalancing after scale-down evicts excess spot pod to satisfy min-on-demand", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, clk := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas leaving 5 spot / 0 on-demand — on-demand count is below minOnDemand=2
		scaleDown(t, cluster, kubernetes.ReplicaSetKind, "default", rs, 5, 0)

		// Rebalancing evicts spot pods until on-demand count reaches minOnDemand=2.
		// Each evicted spot pod is recreated by the ReplicaSet as on-demand.
		for i := range 2 {
			clk.Step(s.Config().RebalanceStabilizationPeriod + epsilon)
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(4-i))

			// ReplicaSet recreates the evicted pod as on-demand (on-demand count still below minOnDemand)
			cluster.CreatePod(newPod("default", kubernetes.ReplicaSetKind, rs))
			cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(i+1))
		}

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=2 satisfied)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
	})

	t.Run("Rebalancing after scale-down evicts excess spot pod (min-on-demand already satisfied)", func(t *testing.T) {
		// Given: minOnDemand=1 so that after scale-down on-demand count can be >= minOnDemand
		// while spot count still exceeds the desired ratio.
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, clk := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 1), 10)
		rs := d.ReplicaSet()
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas leaving 4 spot / 1 on-demand — on-demand satisfies minOnDemand=1
		// but spot count exceeds the desired 3 (60% of 5).
		scaleDown(t, cluster, kubernetes.ReplicaSetKind, "default", rs, 4, 1)

		// When: rebalancing evicts the excess spot pod
		clk.Step(s.Config().RebalanceStabilizationPeriod + epsilon)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))

		// ReplicaSet recreates the evicted pod as on-demand
		cluster.CreatePod(newPod("default", kubernetes.ReplicaSetKind, rs))

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=1 satisfied)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
	})

	t.Run("Rebalancing after scale-down when ratio is already correct", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, clk := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas preserving the ratio — 3 spot / 2 on-demand (60% of 5)
		scaleDown(t, cluster, kubernetes.ReplicaSetKind, "default", rs, 3, 2)

		// Then: ratio is already correct; rebalancing does not evict any pod
		clk.Step(s.Config().RebalanceStabilizationPeriod + epsilon)
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
	})

	t.Run("Pods not eligible for spot are scheduled onto on-demand node", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", nil, nil, 5)

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningOnDemand(5))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningSpot(0))
	})

	t.Run("Pods not eligible for spot are not tracked", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, _ := runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", nil, nil, 5)
		rs := d.ReplicaSet()

		// Then
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(5))
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
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), replicas)
		rs := d.ReplicaSet()

		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		cluster.AssertOwnerPods(kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

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

// scaleDown simulates a Deployment scale-down by deleting pods to reach the expected spot/on-demand counts.
func scaleDown(t *testing.T, cluster *fakeCluster, ownerKind, namespace, name string, expectSpot, expectOnDemand int) {
	t.Helper()
	pods := cluster.ListOwnerPods(ownerKind, namespace, name)

	currentSpot, currentOnDemand := 0, 0
	for _, pod := range pods {
		if spot.IsSpotAssigned(pod) {
			currentSpot++
		} else {
			currentOnDemand++
		}
	}

	require.GreaterOrEqual(t, currentSpot, expectSpot, "expectSpot=%d exceeds current spot count %d", expectSpot, currentSpot)
	require.GreaterOrEqual(t, currentOnDemand, expectOnDemand, "expectOnDemand=%d exceeds current on-demand count %d", expectOnDemand, currentOnDemand)

	spotToDelete := currentSpot - expectSpot
	onDemandToDelete := currentOnDemand - expectOnDemand
	deleted := make(map[string]struct{})
	for _, pod := range pods {
		if spot.IsSpotAssigned(pod) {
			if spotToDelete > 0 {
				cluster.DeletePod(pod)
				deleted[pod.ID] = struct{}{}
				spotToDelete--
			}
		} else {
			if onDemandToDelete > 0 {
				cluster.DeletePod(pod)
				deleted[pod.ID] = struct{}{}
				onDemandToDelete--
			}
		}
	}

	cluster.AssertOwnerPods(ownerKind, namespace, name, expectHasNoneOf(deleted))
	cluster.AssertOwnerPods(ownerKind, namespace, name, expectRunningSpot(expectSpot))
	cluster.AssertOwnerPods(ownerKind, namespace, name, expectRunningOnDemand(expectOnDemand))
}

func expectRunningSpot(count int) func([]*workloadmeta.KubernetesPod) bool {
	return func(pods []*workloadmeta.KubernetesPod) bool {
		actual := 0
		for _, p := range pods {
			if p.Phase == string(corev1.PodRunning) && spot.IsSpotAssigned(p) {
				actual++
			}
		}
		return actual == count
	}
}

func expectRunningOnDemand(count int) func([]*workloadmeta.KubernetesPod) bool {
	return func(pods []*workloadmeta.KubernetesPod) bool {
		actual := 0
		for _, p := range pods {
			if p.Phase == string(corev1.PodRunning) && !spot.IsSpotAssigned(p) {
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
