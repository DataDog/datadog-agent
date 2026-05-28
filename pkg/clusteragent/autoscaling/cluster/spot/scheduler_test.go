// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot_test

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/spot"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// defaultTestConfig is the spot Config used by all test schedulers.
var defaultTestConfig = spot.Config{
	Percentage:                   50,
	MinOnDemandReplicas:          1,
	ScheduleTimeout:              1 * time.Minute,
	FallbackDuration:             2 * time.Minute,
	RebalanceStabilizationPeriod: 1 * time.Minute,
}

// runTestScheduler creates and starts a Scheduler for testing.
func runTestScheduler(ctx context.Context, cluster *fakeCluster) *spot.TestScheduler {
	scheduler := spot.NewTestScheduler(defaultTestConfig, cluster.WLM(), cluster.EvictPodByName, cluster.DynamicClient())
	scheduler.Start(ctx)
	scheduler.WaitSynced()

	cluster.OnPodCreated(scheduler.PodCreated)
	cluster.OnPodDeleted(scheduler.PodDeleted)

	return scheduler
}

// spotEnabledLabels returns the labels required to opt a workload into spot scheduling.
func spotEnabledLabels() map[string]string {
	return map[string]string{spot.SpotEnabledLabelKey: spot.SpotEnabledLabelValue}
}

// spotAnnotations returns annotations to register a spot-enabled workload in the store.
// Pass negative value to omit the specific config field.
func spotAnnotations(percentage, minOnDemand int) map[string]string {
	config := make(map[string]any)
	if percentage >= 0 {
		config["percentage"] = percentage
	}
	if minOnDemand >= 0 {
		config["minOnDemandReplicas"] = minOnDemand
	}
	annotations := make(map[string]string)
	if len(config) > 0 {
		data, _ := json.Marshal(config)
		annotations[spot.SpotConfigAnnotation] = string(data)
	}
	return annotations
}

func TestScenarios(t *testing.T) {
	run := func(name string, f func(t *testing.T)) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				common.ResetMainCtx()
				_, cancelMainCtx := common.GetMainCtxCancel() // WLM uses main context
				defer cancelMainCtx()

				f(t)
			})
		})
	}

	wait := func(d time.Duration) {
		time.Sleep(d)
		synctest.Wait()
	}

	waitABit := func() {
		wait(1 * time.Second)
	}

	waitSchedulerTickAfter := func(d time.Duration) {
		wait(d + spot.SchedulerTick)
	}

	run("Single pod scheduled to on-demand node due to min on-demand replicas constraint", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(100, 2), 1)

		// Then
		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet())
		expectRunningOnDemand(t, pods, 1)
	})

	run("Rolling update preserves ratio", func(t *testing.T) {
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
		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs2)
		expectRunningSpot(t, pods, 6)
		expectRunningOnDemand(t, pods, 4)
	})

	run("Scale-up preserves ratio", func(t *testing.T) {
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
		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet())
		expectRunningOnDemand(t, pods, 2)
		expectRunningSpot(t, pods, 3)

		// When: scale up to 10 replicas
		rs := d.Rollout(labels, annotations, 10)

		// Then: 4 on-demand, 6 spot (60% of 10, minOnDemand=2)
		waitABit()
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningOnDemand(t, pods, 4)
		expectRunningSpot(t, pods, 6)
	})

	run("Changing spot percentage updates ratio", func(t *testing.T) {
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
		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rsName)
		expectRunningOnDemand(t, pods, replicas)

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
			waitABit()
			pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rsName)
			expectRunningOnDemand(t, pods, replicas-step.spot)
			expectRunningSpot(t, pods, step.spot)
		}
	})

	run("Fallback to on-demand when spot node unavailable", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		// No spot node: spot-assigned pods stay Pending.

		s := runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()

		// Then
		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningOnDemand(t, pods, 4)
		expectPending(t, pods, 6)

		// When
		// Schedule timeout
		waitSchedulerTickAfter(s.Config().ScheduleTimeout)

		// Then
		require.True(t, s.IsSpotSchedulingDisabled("apps", kubernetes.DeploymentKind, d.namespace, d.name))

		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningOnDemand(t, pods, 4)
		// Pending pods evicted
		expectPending(t, pods, 0)

		// When
		// ReplicaSet recreates pods
		d.Reconcile()

		// Then
		// Fallback to on-demand
		waitABit()
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningOnDemand(t, pods, 10)

		// When
		cluster.AddSpotNode("new-spot")
		waitSchedulerTickAfter(s.Config().FallbackDuration)

		// Then
		require.False(t, s.IsSpotSchedulingDisabled("apps", kubernetes.DeploymentKind, d.namespace, d.name))

		// Rebalancing: one on-demand pod evicted per stabilization period
		for i := range 6 {
			// Excess on-demand pod evicted
			pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
			expectRunningOnDemand(t, pods, 10-1-i)

			// ReplicaSet recreates pod
			d.Reconcile()
			waitSchedulerTickAfter(s.Config().RebalanceStabilizationPeriod)
		}

		// Then
		waitABit()
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 6)
		expectRunningOnDemand(t, pods, 4)
	})

	run("Pod replacement preserves ratio", func(t *testing.T) {
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

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, expectedSpot)
		expectRunningOnDemand(t, pods, expectedOnDemand)

		for range 10 {
			// When
			// Delete random pods between 1 and len(pods)-1 and
			// create its replacement simulating the ReplicaSet controller.
			pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
			count := rand.N(len(pods)-1) + 1

			deleted := make(map[string]struct{}, count)
			for _, idx := range rand.Perm(len(pods))[:count] {
				pod := pods[idx]

				cluster.DeletePod(pod)
				deleted[pod.ID] = struct{}{}
			}
			// Important: wait until deletion is complete before checking expectations to avoid counting deleted pods.
			waitABit()
			checkPods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
			expectHasNoneOf(t, checkPods, deleted)

			d.Reconcile()

			// Then
			waitABit()
			pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
			expectRunningSpot(t, pods, expectedSpot)
			expectRunningOnDemand(t, pods, expectedOnDemand)
		}
	})

	run("Rebalancing after scale-down evicts excess on-demand pod", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 6)
		expectRunningOnDemand(t, pods, 4)

		// When: scale down to 5 replicas leaving 2 spot / 3 on-demand — ratio is off
		d.ScaleDown(keep(2, 3))

		waitSchedulerTickAfter(s.Config().RebalanceStabilizationPeriod)

		// Then: excess on-demand pod evicted
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningOnDemand(t, pods, 2)

		// ReplicaSet recreates the evicted pod as spot
		d.Reconcile()

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=2 satisfied)
		waitABit()
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 3)
		expectRunningOnDemand(t, pods, 2)
	})

	run("Rebalancing after scale-down evicts excess spot pod to satisfy min-on-demand", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 6)
		expectRunningOnDemand(t, pods, 4)

		// When: scale down to 5 replicas leaving 5 spot / 0 on-demand — on-demand count is below minOnDemand=2
		d.ScaleDown(keep(5, 0))

		// Rebalancing evicts spot pods until on-demand count reaches minOnDemand=2.
		// Each evicted spot pod is recreated by the ReplicaSet as on-demand.
		for i := range 2 {
			// When
			waitSchedulerTickAfter(s.Config().RebalanceStabilizationPeriod)

			// Then: excess spot pod evicted
			pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
			expectRunningSpot(t, pods, 4-i)

			// ReplicaSet recreates the evicted pod as on-demand (on-demand count still below minOnDemand)
			d.Reconcile()
		}

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=2 satisfied)
		waitABit()
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 3)
		expectRunningOnDemand(t, pods, 2)
	})

	run("Rebalancing after scale-down evicts excess spot pod (min-on-demand already satisfied)", func(t *testing.T) {
		// Given: minOnDemand=1 so that after scale-down on-demand count can be >= minOnDemand
		// while spot count still exceeds the desired ratio.
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 1), 10)
		rs := d.ReplicaSet()

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 6)
		expectRunningOnDemand(t, pods, 4)

		// When: scale down to 5 replicas leaving 4 spot / 1 on-demand — on-demand satisfies minOnDemand=1
		// but spot count exceeds the desired 3 (60% of 5).
		d.ScaleDown(keep(4, 1))

		waitSchedulerTickAfter(s.Config().RebalanceStabilizationPeriod)

		// Then: excess spot pod evicted
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 3)

		// ReplicaSet recreates the evicted pod as on-demand
		d.Reconcile()

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=1 satisfied)
		waitABit()
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 3)
		expectRunningOnDemand(t, pods, 2)
	})

	run("Rebalancing after scale-down when ratio is already correct", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 6)
		expectRunningOnDemand(t, pods, 4)

		// When: scale down to 5 replicas preserving the ratio — 3 spot / 2 on-demand (60% of 5)
		d.ScaleDown(keep(3, 2))

		waitSchedulerTickAfter(s.Config().RebalanceStabilizationPeriod)

		// Then: ratio is already correct; rebalancing does not evict any pod
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 3)
		expectRunningOnDemand(t, pods, 2)
	})

	run("Pods not eligible for spot are scheduled onto on-demand node", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", nil, nil, 5)

		// Then
		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", d.ReplicaSet())
		expectRunningOnDemand(t, pods, 5)
		expectRunningSpot(t, pods, 0)
	})

	run("Opt-in to spot scheduling after initial deployment converges via rebalancing", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		const replicas = 10
		const spotPercentage = 60
		const expectedSpot = 6

		// When: create deployment without spot label — all pods go on-demand
		d := cluster.CreateDeployment("default", "nginx", nil, nil, replicas)
		rs := d.ReplicaSet()

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningOnDemand(t, pods, replicas)

		// When: opt in to spot scheduling (no new pods created)
		d.UpdateMetadata(spotEnabledLabels(), spotAnnotations(spotPercentage, 2))

		waitABit()

		total, _ := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		require.Equal(t, replicas, total)

		// Then: rebalancer evicts one on-demand pod per cycle; RS recreates it as spot.
		for i := range expectedSpot {
			// When
			waitSchedulerTickAfter(s.Config().RebalanceStabilizationPeriod)

			// Then: excess on-demand pod evicted
			pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
			expectRunningOnDemand(t, pods, replicas-1-i)

			d.Reconcile()
		}

		waitABit()
		pods = cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, expectedSpot)
		expectRunningOnDemand(t, pods, replicas-expectedSpot)
	})

	run("Opt-out via label removal clears config store and pod tracker", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		const replicas = 5
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 1), replicas)
		rs := d.ReplicaSet()

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 3)

		require.True(t, s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name))
		total, _ := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		require.Equal(t, replicas, total)

		// When: remove the spot label (opt-out)
		d.RemoveLabels(spot.SpotEnabledLabelKey)

		// Then: config store and pod tracker are cleared
		waitABit()
		require.False(t, s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name))
		require.False(t, s.HasTrackedPods("apps", kubernetes.DeploymentKind, d.namespace, d.name))
	})

	run("Opt-out via deployment deletion clears config store and pod tracker", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		const replicas = 5
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 1), replicas)
		rs := d.ReplicaSet()

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 3)

		require.True(t, s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name))
		total, _ := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		require.Equal(t, replicas, total)

		// When: delete the deployment and its pods
		d.Delete()

		// Then: config store and pod tracker are cleared
		waitABit()
		require.False(t, s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name))
		require.False(t, s.HasTrackedPods("apps", kubernetes.DeploymentKind, d.namespace, d.name))
	})

	run("Pods not eligible for spot are not tracked", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s := runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", nil, nil, 5)
		rs := d.ReplicaSet()

		// Then
		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningOnDemand(t, pods, 5)
		total, spotCount := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		assert.Zero(t, total)
		assert.Zero(t, spotCount)
	})

	run("Restarted scheduler tracks existing pods", func(t *testing.T) {
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

		waitABit()
		pods := cluster.ListOwnerPods(kubernetes.ReplicaSetKind, "default", rs)
		expectRunningSpot(t, pods, 6)
		expectRunningOnDemand(t, pods, 4)

		// When
		stopScheduler()

		s2 := runTestScheduler(t.Context(), cluster)

		// Then
		waitABit()
		total, spotCount := s2.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		require.Equal(t, replicas, total)
		require.Equal(t, 6, spotCount)
	})
}

// keep returns a filter that retains given number of spot and on-demand pods.
func keep(spotCount, onDemandCount int) func([]*workloadmeta.KubernetesPod) []*workloadmeta.KubernetesPod {
	return func(pods []*workloadmeta.KubernetesPod) []*workloadmeta.KubernetesPod {
		var toDelete []*workloadmeta.KubernetesPod
		for _, pod := range pods {
			if spot.IsSpotAssigned(pod) {
				if spotCount > 0 {
					spotCount--
				} else {
					toDelete = append(toDelete, pod)
				}
			} else {
				if onDemandCount > 0 {
					onDemandCount--
				} else {
					toDelete = append(toDelete, pod)
				}
			}
		}
		return toDelete
	}
}

// groupPods groups pods by capacity type ("spot"/"on-demand") then by phase,
// returning a map suitable for use in assertion messages.
func groupPods(pods []*workloadmeta.KubernetesPod) map[string]map[corev1.PodPhase][]string {
	g := make(map[string]map[corev1.PodPhase][]string)
	for _, p := range pods {
		capacityType := "on-demand"
		if spot.IsSpotAssigned(p) {
			capacityType = "spot"
		}
		if g[capacityType] == nil {
			g[capacityType] = make(map[corev1.PodPhase][]string)
		}
		phase := corev1.PodPhase(p.Phase)
		g[capacityType][phase] = append(g[capacityType][phase], p.Name)
	}
	return g
}

func expectRunningSpot(t *testing.T, pods []*workloadmeta.KubernetesPod, count int) {
	t.Helper()
	g := groupPods(pods)
	require.Equal(t, count, len(g["spot"][corev1.PodRunning]), "expected %d running spot pods; pod breakdown: %v", count, g)
}

func expectRunningOnDemand(t *testing.T, pods []*workloadmeta.KubernetesPod, count int) {
	t.Helper()
	g := groupPods(pods)
	require.Equal(t, count, len(g["on-demand"][corev1.PodRunning]), "expected %d running on-demand pods; pod breakdown: %v", count, g)
}

func expectPending(t *testing.T, pods []*workloadmeta.KubernetesPod, count int) {
	t.Helper()
	g := groupPods(pods)
	actual := len(g["spot"][corev1.PodPending]) + len(g["on-demand"][corev1.PodPending])
	require.Equal(t, count, actual, "expected %d pending pods; pod breakdown: %v", count, g)
}

func expectHasNoneOf(t *testing.T, pods []*workloadmeta.KubernetesPod, ids map[string]struct{}) {
	t.Helper()
	var found []string
	for _, pod := range pods {
		if _, ok := ids[pod.ID]; ok {
			found = append(found, pod.ID)
		}
	}
	require.Empty(t, found, "deleted pod IDs still present: %v; pod breakdown: %v", found, groupPods(pods))
}
