// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package spot

import (
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TestStatefulSetSinglePodMinOnDemand: 1 replica with minOnDemand=2 → pod goes on-demand.
func (s *spotSchedulingSuite) TestStatefulSetSinglePodMinOnDemand() {
	s.createStatefulSet("redis", 1, &spotConfig{spotPercentage: 100, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, 1)
		s.expectRunningOnDemand(c, pods, 1)
	})
}

// TestStatefulSetSpotPercentage: 10 replicas at 60% spot, minOnDemand=2 → 6 spot, 4 on-demand.
func (s *spotSchedulingSuite) TestStatefulSetSpotPercentage() {
	s.createStatefulSet("redis", 10, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})
}

// TestStatefulSetNonEligiblePodsNotModified: StatefulSet without spot label → pods go on-demand, no spot assignment.
func (s *spotSchedulingSuite) TestStatefulSetNonEligiblePodsNotModified() {
	s.createStatefulSet("redis", 3, nil)

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, 3)
		s.expectRunningOnDemand(c, pods, 3)
		for _, p := range pods {
			require.NotContains(c, p.Labels, spotAssignedLabel,
				"pod %s should not have spot-assigned label: statefulset is not spot-eligible", p.Name)
		}
	})
}

// TestStatefulSetFallbackWhenSpotUnavailable: cordon spot node so spot-assigned pods stay Pending,
// verify the scheduler disables spot and re-creates pods on on-demand.
// Then uncordon node and verify pods re-balanced to spot node.
func (s *spotSchedulingSuite) TestStatefulSetFallbackWhenSpotUnavailable() {
	s.cordonNode(s.spotNode)

	s.createStatefulSet("redis", 10, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// After ScheduleTimeout (30s), the scheduler should:
	// 1. Set spot-disabled-until annotation on StatefulSet
	// 2. Evict pending spot pods
	// 3. New pods from StatefulSet go to on-demand node
	var disabledUntil time.Time
	s.eventually(func(c *assert.CollectT) {
		sts, err := s.kubeClient.AppsV1().StatefulSets(s.testNamespace).Get(s.T().Context(), "redis", metav1.GetOptions{})
		require.NoError(c, err)
		require.NotEmpty(c, sts.Annotations[spotDisabledUntilAnnotation],
			"spot-disabled-until annotation should be set on StatefulSet after fallback")

		disabledUntil, err = time.Parse(time.RFC3339, sts.Annotations[spotDisabledUntilAnnotation])
		s.Require().NoError(err)
	})

	// Wait for all 10 pods to run on on-demand (fallback mode).
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, 10)
		s.expectRunningOnDemand(c, pods, 10)
	})

	// Uncordon and wait for all 10 pods to rebalance to 6 spot / 4 on-demand.
	s.uncordonNode(s.spotNode)

	fallbackWait := max(0, time.Until(disabledUntil))
	s.T().Logf("Remaining fallback duration: %v", fallbackWait)

	const spotPods = 6 // 60% of 10 replicas
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, spotPods)
		s.expectRunningOnDemand(c, pods, 4)
	}, fallbackWait+rebalancingTimeout(spotPods), 5*time.Second)
}

// TestStatefulSetOptIn: create a statefulset without the spot label, wait for all pods
// to run on-demand, then opt in by adding the spot-enabled label and annotations.
// Verify the statefulset converges to the desired spot ratio via rebalancing.
func (s *spotSchedulingSuite) TestStatefulSetOptIn() {
	const replicas = 10

	// Step 1: create statefulset without spot label — all pods go on-demand.
	s.createStatefulSet("redis", replicas, nil)

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, replicas)
		s.expectRunningOnDemand(c, pods, replicas)
	})

	// Step 2: opt in to spot scheduling.
	s.updateStatefulSet("redis", &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// Step 3: expect rebalancing to converge to 6 spot / 4 on-demand.
	const spotPods = 6 // 60% of 10 replicas
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, spotPods)
		s.expectRunningOnDemand(c, pods, replicas-spotPods)
	}, rebalancingTimeout(spotPods), 5*time.Second)
}

// TestStatefulSetRebalancingAfterScaleDown: after scaling down a statefulset, verify the rebalancer
// evicts excess pods to restore the correct spot/on-demand ratio.
func (s *spotSchedulingSuite) TestStatefulSetRebalancingAfterScaleDown() {
	s.createStatefulSet("redis", 10, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// Wait for initial 6 spot / 4 on-demand.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})

	// Scale the StatefulSet down to 5 replicas.
	s.scaleStatefulSet("redis", 5)

	// Wait for rebalancing.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")

		require.Len(c, pods, 5)
		s.expectRunningSpot(c, pods, 3)
		s.expectRunningOnDemand(c, pods, 2)
	})
}

// TestStatefulSetRollingUpdatePreservesRatio: after triggering a rolling update the
// new pods must converge to the same spot/on-demand ratio.
// Unlike Deployments, StatefulSet rolling update replaces pods in-place under the
// same owner (no new ReplicaSet is created).
func (s *spotSchedulingSuite) TestStatefulSetRollingUpdatePreservesRatio() {
	const replicas = 10

	s.createStatefulSet("redis", replicas, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")
		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})

	// Trigger a rolling update by bumping the pod-template annotation.
	s.rolloutRestartStatefulSet("redis")

	// After the rollout the pods should still converge to 6 spot / 4 on-demand.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")
		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})
}

// TestStatefulSetScaleUpPreservesRatio: scaling up from 5 to 10 replicas must maintain
// the configured spot/on-demand ratio for the additional pods.
func (s *spotSchedulingSuite) TestStatefulSetScaleUpPreservesRatio() {
	s.createStatefulSet("redis", 5, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// Initial: minOnDemand=2 → 2 on-demand, 3 spot (60% of 5).
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")
		require.Len(c, pods, 5)
		s.expectRunningSpot(c, pods, 3)
		s.expectRunningOnDemand(c, pods, 2)
	})

	// Scale up to 10 replicas: expect 6 spot / 4 on-demand.
	s.scaleStatefulSet("redis", 10)

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")
		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})
}

// TestStatefulSetOptOut: remove the spot-enabled label and restart the StatefulSet;
// all replacement pods must land on the on-demand node with no spot-assigned label.
func (s *spotSchedulingSuite) TestStatefulSetOptOut() {
	const replicas = 10

	// Step 1: opt in — wait for 6 spot / 4 on-demand.
	s.createStatefulSet("redis", replicas, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")
		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})

	// Step 2: remove spot label to opt out, then restart to replace existing pods.
	s.removeStatefulSetLabel("redis", spotEnabledLabelKey)
	s.rolloutRestartStatefulSet("redis")

	// Step 3: all pods should land on on-demand without the spot-assigned label.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("statefulset=redis")
		require.Len(c, pods, replicas)
		s.expectRunningOnDemand(c, pods, replicas)
		for _, p := range pods {
			require.NotContains(c, p.Labels, spotAssignedLabel,
				"pod %s should not have spot-assigned label after opt-out", p.Name)
		}
	})
}

// createStatefulSet creates a StatefulSet with a pause container and a "statefulset=name" selector
// label on the pod template. If config is non-nil, the spot-enabled label and spot-config
// annotation are added to opt the statefulset in to spot scheduling.
func (s *spotSchedulingSuite) createStatefulSet(name string, replicas int32, config *spotConfig) {
	s.T().Helper()

	podSelector := map[string]string{"statefulset": name}

	stsLabels := map[string]string{"statefulset": name}
	var annotations map[string]string
	if config != nil {
		stsLabels[spotEnabledLabelKey] = spotEnabledLabelValue
		annotations = map[string]string{
			spotConfigAnnotation: fmt.Sprintf(`{"percentage":%d,"minOnDemandReplicas":%d}`, config.spotPercentage, config.minOnDemandReplicas),
		}
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   s.testNamespace,
			Labels:      stsLabels,
			Annotations: annotations,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: podSelector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podSelector,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "registry.k8s.io/pause",
					}},
				},
			},
		},
	}
	_, err := s.kubeClient.AppsV1().StatefulSets(s.testNamespace).Create(s.T().Context(), sts, metav1.CreateOptions{})
	s.Require().NoError(err)
}

// updateStatefulSet applies the spot-enabled label and spot-config annotation from config
// using a merge patch to avoid optimistic concurrency conflicts.
func (s *spotSchedulingSuite) updateStatefulSet(name string, config *spotConfig) {
	s.T().Helper()
	ctx := s.T().Context()

	configValue := fmt.Sprintf(`{"percentage":%d,"minOnDemandReplicas":%d}`, config.spotPercentage, config.minOnDemandReplicas)
	patch := fmt.Sprintf(`{"metadata":{"labels":{%q:%q},"annotations":{%q:%q}}}`, spotEnabledLabelKey, spotEnabledLabelValue, spotConfigAnnotation, configValue)

	_, err := s.kubeClient.AppsV1().StatefulSets(s.testNamespace).Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}

// scaleStatefulSet updates the replica count of a StatefulSet
// using a merge patch to avoid optimistic concurrency conflicts.
func (s *spotSchedulingSuite) scaleStatefulSet(name string, replicas int32) {
	s.T().Helper()
	ctx := s.T().Context()

	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)

	_, err := s.kubeClient.AppsV1().StatefulSets(s.testNamespace).Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}

// rolloutRestartStatefulSet triggers a rolling update by patching the pod-template restart annotation.
func (s *spotSchedulingSuite) rolloutRestartStatefulSet(name string) {
	s.T().Helper()
	ctx := s.T().Context()

	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`, time.Now().UTC().Format(time.RFC3339))

	_, err := s.kubeClient.AppsV1().StatefulSets(s.testNamespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}

// removeStatefulSetLabel removes a single label key from a StatefulSet.
func (s *spotSchedulingSuite) removeStatefulSetLabel(name, key string) {
	s.T().Helper()
	ctx := s.T().Context()

	// JSON Pointer (RFC 6901) requires escaping: ~ → ~0, / → ~1.
	escaped := strings.NewReplacer("~", "~0", "/", "~1").Replace(key)
	patch := fmt.Sprintf(`[{"op":"remove","path":"/metadata/labels/%s"}]`, escaped)

	_, err := s.kubeClient.AppsV1().StatefulSets(s.testNamespace).Patch(ctx, name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}
