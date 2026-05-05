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

type spotConfig struct {
	spotPercentage      int
	minOnDemandReplicas int
}

// TestDeploymentSinglePodMinOnDemand: 1 replica with minOnDemand=2 → pod goes on-demand.
func (s *spotSchedulingSuite) TestDeploymentSinglePodMinOnDemand() {
	s.createDeployment("nginx", 1, &spotConfig{spotPercentage: 100, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, 1)
		s.expectRunningOnDemand(c, pods, 1)
	})
}

// TestDeploymentSpotPercentage: 10 replicas at 60% spot, minOnDemand=2 → 6 spot, 4 on-demand.
func (s *spotSchedulingSuite) TestDeploymentSpotPercentage() {
	s.createDeployment("nginx", 10, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})
}

// TestDeploymentNonEligiblePodsNotModified: Deployment without spot label → pods go on-demand, no spot assignment.
func (s *spotSchedulingSuite) TestDeploymentNonEligiblePodsNotModified() {
	s.createDeployment("nginx", 3, nil)

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, 3)
		s.expectRunningOnDemand(c, pods, 3)
		for _, p := range pods {
			require.NotContains(c, p.Labels, spotAssignedLabel,
				"pod %s should not have spot-assigned label: deployment is not spot-eligible", p.Name)
		}
	})
}

// TestDeploymentFallbackWhenSpotUnavailable: cordon spot node so spot-assigned pods stay Pending,
// verify the scheduler disables spot and re-creates pods on on-demand.
// Then uncordon node and verify pods re-balanced to spot node.
func (s *spotSchedulingSuite) TestDeploymentFallbackWhenSpotUnavailable() {
	// Cordon spot node so spot-assigned pods (which have nodeSelector=spot) stay Pending.
	s.cordonNode(s.spotNode)

	s.createDeployment("nginx", 10, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// Do not wait for 4 on-demand pods Running + 6 spot-assigned pods Pending.
	// as there is a race between Deployment creation and Webhook such that
	// all pods may land to on-demand nodes initially.
	// Scheduler will rebalance this Deployment by evicting one pod at a time and
	// replacement pod will be assigned to spot and become Pending.
	// Therefore it may never reach 6 Pending spot pods before on-demand fallback kicks in so
	// wait for the fallback marker instead.

	// After ScheduleTimeout (30s), the scheduler should:
	// 1. Set spot-disabled-until annotation on Deployment
	// 2. Evict pending spot pods
	// 3. New pods from ReplicaSet go to on-demand node
	var lastDisabledUntil string
	s.eventually(func(c *assert.CollectT) {
		deploy, err := s.kubeClient.AppsV1().Deployments(s.testNamespace).Get(s.T().Context(), "nginx", metav1.GetOptions{})
		require.NoError(c, err)
		require.NotEmpty(c, deploy.Annotations[spotDisabledUntilAnnotation],
			"spot-disabled-until annotation should be set on Deployment after fallback")

		lastDisabledUntil = deploy.Annotations[spotDisabledUntilAnnotation]
	})

	// Wait for all 10 pods to run on on-demand (fallback mode).
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, 10)
		s.expectRunningOnDemand(c, pods, 10)
	})

	// Uncordon and wait for all 10 pods to run on spot and on-demand due to rebalancing.
	s.uncordonNode(s.spotNode)

	const spotPods = 6 // 60% of 10 replicas
	fallbackCount := 1
	s.EventuallyWithT(func(c *assert.CollectT) {
		deploy, err := s.kubeClient.AppsV1().Deployments(s.testNamespace).Get(s.T().Context(), "nginx", metav1.GetOptions{})
		require.NoError(c, err)
		disabledUntil := deploy.Annotations[spotDisabledUntilAnnotation]
		if disabledUntil != "" && disabledUntil != lastDisabledUntil {
			fallbackCount++
			lastDisabledUntil = disabledUntil
			s.T().Logf("spot fallback #%d, disabled until: %s", fallbackCount, disabledUntil)
		}

		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, spotPods)
		s.expectRunningOnDemand(c, pods, 4)
	}, 2*fallbackDuration+rebalancingTimeout(spotPods), 5*time.Second)
}

// TestDeploymentOptIn: create a deployment without the spot label, wait for all pods
// to run on-demand, then opt in by adding the spot-enabled label and annotations.
// Verify the deployment converges to the desired spot ratio via rebalancing.
func (s *spotSchedulingSuite) TestDeploymentOptIn() {
	const replicas = 10

	// Step 1: create deployment without spot label — all pods go on-demand.
	s.createDeployment("nginx", replicas, nil)

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, replicas)
		s.expectRunningOnDemand(c, pods, replicas)
	})

	// Step 2: opt in to spot scheduling.
	s.updateDeployment("nginx", &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// Step 3: expect rebalancing to converge to 6 spot / 4 on-demand.
	// Budget: one rebalance cycle per spot pod to be evicted and replaced.
	const spotPods = 6 // 60% of 10 replicas
	s.EventuallyWithT(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, spotPods)
		s.expectRunningOnDemand(c, pods, replicas-spotPods)
	}, rebalancingTimeout(spotPods), 5*time.Second)
}

// TestDeploymentRebalancingAfterScaleDown: after scaling down a deployment, verify the rebalancer
// evicts excess pods to restore the correct spot/on-demand ratio.
func (s *spotSchedulingSuite) TestDeploymentRebalancingAfterScaleDown() {
	s.createDeployment("nginx", 10, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// Wait for initial 6 spot / 4 on-demand.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})

	// Scale the Deployment down to 5 replicas.
	s.scaleDeployment("nginx", 5)

	// Wait for rebalancing.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")

		require.Len(c, pods, 5)
		s.expectRunningSpot(c, pods, 3)
		s.expectRunningOnDemand(c, pods, 2)
	})
}

// TestDeploymentRollingUpdatePreservesRatio: after triggering a rolling update the new
// ReplicaSet's pods must converge to the same spot/on-demand ratio.
func (s *spotSchedulingSuite) TestDeploymentRollingUpdatePreservesRatio() {
	const replicas = 10

	s.createDeployment("nginx", replicas, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")
		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})

	// Trigger a rolling update by bumping the pod-template annotation.
	s.rolloutRestart("nginx")

	// After the rollout the new ReplicaSet's pods should also converge to 6 spot / 4 on-demand.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")
		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})
}

// TestDeploymentScaleUpPreservesRatio: scaling up from 5 to 10 replicas must maintain
// the configured spot/on-demand ratio for the additional pods.
func (s *spotSchedulingSuite) TestDeploymentScaleUpPreservesRatio() {
	s.createDeployment("nginx", 5, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	// Initial: minOnDemand=2 → 2 on-demand, 3 spot (60% of 5).
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")
		require.Len(c, pods, 5)
		s.expectRunningSpot(c, pods, 3)
		s.expectRunningOnDemand(c, pods, 2)
	})

	// Scale up to 10 replicas: expect 6 spot / 4 on-demand.
	s.scaleDeployment("nginx", 10)

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")
		require.Len(c, pods, 10)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})
}

// TestDeploymentOptOut: remove the spot-enabled label and restart the Deployment;
// all replacement pods must land on the on-demand node with no spot-assigned label.
func (s *spotSchedulingSuite) TestDeploymentOptOut() {
	const replicas = 10

	// Step 1: opt in — wait for 6 spot / 4 on-demand.
	s.createDeployment("nginx", replicas, &spotConfig{spotPercentage: 60, minOnDemandReplicas: 2})

	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")
		require.Len(c, pods, replicas)
		s.expectRunningSpot(c, pods, 6)
		s.expectRunningOnDemand(c, pods, 4)
	})

	// Step 2: remove spot label to opt out, then restart to replace existing pods.
	s.removeDeploymentLabel("nginx", spotEnabledLabelKey)
	s.rolloutRestart("nginx")

	// Step 3: all pods should land on on-demand without the spot-assigned label.
	s.eventually(func(c *assert.CollectT) {
		pods := s.listPods("deployment=nginx")
		require.Len(c, pods, replicas)
		s.expectRunningOnDemand(c, pods, replicas)
		for _, p := range pods {
			require.NotContains(c, p.Labels, spotAssignedLabel,
				"pod %s should not have spot-assigned label after opt-out", p.Name)
		}
	})
}

// createDeployment creates a Deployment with a pause container and a "deployment=name" selector
// label on the pod template. If config is non-nil, the spot-enabled label and spot-config
// annotation are added to opt the deployment in to spot scheduling.
func (s *spotSchedulingSuite) createDeployment(name string, replicas int32, config *spotConfig) {
	s.T().Helper()

	podSelector := map[string]string{"deployment": name}

	deployLabels := map[string]string{"deployment": name}
	var annotations map[string]string
	if config != nil {
		deployLabels[spotEnabledLabelKey] = spotEnabledLabelValue
		annotations = map[string]string{
			spotConfigAnnotation: fmt.Sprintf(`{"percentage":%d,"minOnDemandReplicas":%d}`, config.spotPercentage, config.minOnDemandReplicas),
		}
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   s.testNamespace,
			Labels:      deployLabels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
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
	_, err := s.kubeClient.AppsV1().Deployments(s.testNamespace).Create(s.T().Context(), deploy, metav1.CreateOptions{})
	s.Require().NoError(err)
}

// updateDeployment applies the spot-enabled label and spot-config annotation from config
// using a merge patch to avoid optimistic concurrency conflicts.
func (s *spotSchedulingSuite) updateDeployment(name string, config *spotConfig) {
	s.T().Helper()
	ctx := s.T().Context()

	configValue := fmt.Sprintf(`{"percentage":%d,"minOnDemandReplicas":%d}`, config.spotPercentage, config.minOnDemandReplicas)
	patch := fmt.Sprintf(`{"metadata":{"labels":{%q:%q},"annotations":{%q:%q}}}`, spotEnabledLabelKey, spotEnabledLabelValue, spotConfigAnnotation, configValue)

	_, err := s.kubeClient.AppsV1().Deployments(s.testNamespace).Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}

// scaleDeployment updates the replica count of a Deployment
// using a merge patch to avoid optimistic concurrency conflicts.
func (s *spotSchedulingSuite) scaleDeployment(name string, replicas int32) {
	s.T().Helper()
	ctx := s.T().Context()

	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)

	_, err := s.kubeClient.AppsV1().Deployments(s.testNamespace).Patch(ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}

// rolloutRestart triggers a rolling update by patching the pod-template restart annotation.
func (s *spotSchedulingSuite) rolloutRestart(name string) {
	s.T().Helper()
	ctx := s.T().Context()

	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`, time.Now().UTC().Format(time.RFC3339))

	_, err := s.kubeClient.AppsV1().Deployments(s.testNamespace).Patch(ctx, name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}

// removeDeploymentLabel removes a single label key from a Deployment.
func (s *spotSchedulingSuite) removeDeploymentLabel(name, key string) {
	s.T().Helper()
	ctx := s.T().Context()

	// JSON Pointer (RFC 6901) requires escaping: ~ → ~0, / → ~1.
	escaped := strings.NewReplacer("~", "~0", "/", "~1").Replace(key)
	patch := fmt.Sprintf(`[{"op":"remove","path":"/metadata/labels/%s"}]`, escaped)

	_, err := s.kubeClient.AppsV1().Deployments(s.testNamespace).Patch(ctx, name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
	s.Require().NoError(err)
}
