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
	"sync/atomic"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	clocktesting "k8s.io/utils/clock/testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	spot "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster/spot"
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
// Returns the scheduler and a fake clock.
func runTestScheduler(ctx context.Context, cluster *fakeCluster) (*spot.TestScheduler, *clocktesting.FakeClock) {
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
	t.Run("Single pod scheduled to on-demand node due to min on-demand replicas constraint", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		runTestScheduler(t.Context(), cluster)

		// When
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(100, 2), 1)

		// Then
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningOnDemand(1))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs2, expectRunningSpot(6))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs2, expectRunningOnDemand(4))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningOnDemand(2))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningSpot(3))

		// When: scale up to 10 replicas
		rs := d.Rollout(labels, annotations, 10)

		// Then: 4 on-demand, 6 spot (60% of 10, minOnDemand=2)
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rsName, expectRunningOnDemand(replicas))

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
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rsName, expectRunningOnDemand(replicas-step.spot))
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rsName, expectRunningSpot(step.spot))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectPending(6))

		// When
		// Schedule timeout
		clk.Step(s.Config().ScheduleTimeout)

		// Then
		requireEventually(t, func() bool {
			return s.IsSpotSchedulingDisabled("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})

		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
		// Pending pods evicted
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectPending(0))

		// When
		// ReplicaSet recreates pods
		d.Reconcile()

		// Then
		// Fallback to on-demand
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(10))

		// When
		cluster.AddSpotNode("new-spot")

		// Advance past disabled interval to re-enable spot scheduling.
		stepClockAfterUpdatesSettled(t, s, clk, s.Config().FallbackDuration, "apps", kubernetes.DeploymentKind, d.namespace, d.name)

		requireEventually(t, func() bool {
			return !s.IsSpotSchedulingDisabled("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})

		// Rebalancing
		for i := range 6 {
			// When
			stepClockAfterUpdatesSettled(t, s, clk, s.Config().RebalanceStabilizationPeriod, "apps", kubernetes.DeploymentKind, d.namespace, d.name)

			// Then: excess on-demand pod evicted
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(10-1-i))

			// ReplicaSet recreates pod
			d.Reconcile()
		}

		// Then
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(expectedSpot))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(expectedOnDemand))

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
			}
			// Important: wait until deletion is complete before checking expectations to avoid counting deleted pods.
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectHasNoneOf(deleted))

			d.Reconcile()

			// Then
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(expectedSpot))
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(expectedOnDemand))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas leaving 2 spot / 3 on-demand — ratio is off
		d.ScaleDown(keep(2, 3))

		stepClockAfterUpdatesSettled(t, s, clk, s.Config().RebalanceStabilizationPeriod, "apps", kubernetes.DeploymentKind, d.namespace, d.name)

		// Then: excess on-demand pod evicted
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))

		// ReplicaSet recreates the evicted pod as spot
		d.Reconcile()

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=2 satisfied)
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
	})

	t.Run("Rebalancing after scale-down evicts excess spot pod to satisfy min-on-demand", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, clk := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas leaving 5 spot / 0 on-demand — on-demand count is below minOnDemand=2
		d.ScaleDown(keep(5, 0))

		// Rebalancing evicts spot pods until on-demand count reaches minOnDemand=2.
		// Each evicted spot pod is recreated by the ReplicaSet as on-demand.
		for i := range 2 {
			// When
			stepClockAfterUpdatesSettled(t, s, clk, s.Config().RebalanceStabilizationPeriod, "apps", kubernetes.DeploymentKind, d.namespace, d.name)

			// Then: excess spot pod evicted
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(4-i))

			// ReplicaSet recreates the evicted pod as on-demand (on-demand count still below minOnDemand)
			d.Reconcile()
		}

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=2 satisfied)
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas leaving 4 spot / 1 on-demand — on-demand satisfies minOnDemand=1
		// but spot count exceeds the desired 3 (60% of 5).
		d.ScaleDown(keep(4, 1))

		stepClockAfterUpdatesSettled(t, s, clk, s.Config().RebalanceStabilizationPeriod, "apps", kubernetes.DeploymentKind, d.namespace, d.name)

		// Then: excess spot pod evicted
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))

		// ReplicaSet recreates the evicted pod as on-demand
		d.Reconcile()

		// Then: 3 spot / 2 on-demand (60% of 5, minOnDemand=1 satisfied)
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
	})

	t.Run("Rebalancing after scale-down when ratio is already correct", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, clk := runTestScheduler(t.Context(), cluster)

		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 2), 10)
		rs := d.ReplicaSet()
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When: scale down to 5 replicas preserving the ratio — 3 spot / 2 on-demand (60% of 5)
		d.ScaleDown(keep(3, 2))

		stepClockAfterUpdatesSettled(t, s, clk, s.Config().RebalanceStabilizationPeriod, "apps", kubernetes.DeploymentKind, d.namespace, d.name)

		// Then: ratio is already correct; rebalancing does not evict any pod
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(2))
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningOnDemand(5))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", d.ReplicaSet(), expectRunningSpot(0))
	})

	t.Run("Opt-in to spot scheduling after initial deployment converges via rebalancing", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, clk := runTestScheduler(t.Context(), cluster)

		const replicas = 10
		const spotPercentage = 60
		const expectedSpot = 6

		// When: create deployment without spot label — all pods go on-demand
		d := cluster.CreateDeployment("default", "nginx", nil, nil, replicas)
		rs := d.ReplicaSet()
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(replicas))

		// When: opt in to spot scheduling (no new pods created)
		d.UpdateMetadata(spotEnabledLabels(), spotAnnotations(spotPercentage, 2))

		// Wait for pod fetcher backfill: tracker must know all 10 pods before advancing the clock.
		requireEventually(t, func() bool {
			total, _ := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
			return total == replicas
		})

		// Then: rebalancer evicts one on-demand pod per cycle; RS recreates it as spot.
		for i := range expectedSpot {
			// When
			stepClockAfterUpdatesSettled(t, s, clk, s.Config().RebalanceStabilizationPeriod, "apps", kubernetes.DeploymentKind, d.namespace, d.name)

			// Then: excess on-demand pod evicted
			requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(replicas-1-i))

			d.Reconcile()
		}

		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(expectedSpot))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(replicas-expectedSpot))
	})

	t.Run("Opt-out via label removal clears config store and pod tracker", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, _ := runTestScheduler(t.Context(), cluster)

		const replicas = 5
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 1), replicas)
		rs := d.ReplicaSet()
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))

		requireEventually(t, func() bool {
			return s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})
		requireEventually(t, func() bool {
			total, _ := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
			return total == replicas
		})

		// When: remove the spot label (opt-out)
		d.RemoveLabels(spot.SpotEnabledLabelKey)

		// Then: config store and pod tracker are cleared
		requireEventually(t, func() bool {
			return !s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})
		requireEventually(t, func() bool {
			return !s.HasTrackedPods("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})
	})

	t.Run("Opt-out via deployment deletion clears config store and pod tracker", func(t *testing.T) {
		// Given
		cluster := newFakeCluster(t)
		cluster.AddOnDemandNode("on-demand")
		cluster.AddSpotNode("spot")

		s, _ := runTestScheduler(t.Context(), cluster)

		const replicas = 5
		d := cluster.CreateDeployment("default", "nginx", spotEnabledLabels(), spotAnnotations(60, 1), replicas)
		rs := d.ReplicaSet()
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(3))

		requireEventually(t, func() bool {
			return s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})
		requireEventually(t, func() bool {
			total, _ := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
			return total == replicas
		})

		// When: delete the deployment and its pods
		d.Delete()

		// Then: config store and pod tracker are cleared
		requireEventually(t, func() bool {
			return !s.HasConfig("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})
		requireEventually(t, func() bool {
			return !s.HasTrackedPods("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		})
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
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(5))
		total, spot := s.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
		assert.Zero(t, total)
		assert.Zero(t, spot)
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

		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningSpot(6))
		requireOwnerPods(cluster, kubernetes.ReplicaSetKind, "default", rs, expectRunningOnDemand(4))

		// When
		stopScheduler()

		s2, _ := runTestScheduler(t.Context(), cluster)

		// Then
		requireEventually(t, func() bool {
			total, spotCount := s2.TrackedCounts("apps", kubernetes.DeploymentKind, d.namespace, d.name)
			return total == replicas && spotCount == 6
		})
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

// stepClockAfterUpdatesSettled waits for the pod tracker to have no in-flight admissions or pending pods
// for the given workload, then advances the fake clock by duration.
func stepClockAfterUpdatesSettled(t *testing.T, s *spot.TestScheduler, clk *clocktesting.FakeClock, duration time.Duration, group, kind, namespace, name string) {
	t.Helper()
	requireEventually(t, func() bool {
		return !s.HasAdmissionsOrPending(group, kind, namespace, name)
	})
	clk.Step(duration)
}

func requireEventually(t *testing.T, condition func() bool, msgAndArgs ...any) {
	t.Helper()
	const (
		waitFor = 5 * time.Second
		tick    = 100 * time.Millisecond
	)
	require.Eventually(t, condition, waitFor, tick, msgAndArgs...)
}

type spewStringer[T any] struct {
	v atomic.Pointer[T]
}

func (h *spewStringer[T]) set(v T) T {
	h.v.Store(&v)
	return v
}

func (h *spewStringer[T]) String() string {
	if p := h.v.Load(); p != nil {
		return spew.Sdump(*p)
	}
	return "<nil>"
}

// requireOwnerPods checks that all pods owned by ownerKind/namespace/ownerName eventually satisfy check.
func requireOwnerPods(c *fakeCluster, ownerKind, namespace, ownerName string, check func(wlm []*workloadmeta.KubernetesPod) bool) {
	c.T().Helper()

	pods := new(spewStringer[[]*workloadmeta.KubernetesPod])
	requireEventually(c.T(), func() bool {
		return check(pods.set(c.ListOwnerPods(ownerKind, namespace, ownerName)))
	}, "%s %s/%s, pods: %s", ownerKind, namespace, ownerName, pods)
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
