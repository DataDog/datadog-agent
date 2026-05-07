// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package spot contains end-to-end tests for the cluster-agent spot scheduler.
// Tests run against a real kind cluster with 1 control-plane, 1 on-demand worker,
// and 1 spot worker. The full cluster-agent is deployed via Helm; no mocking is used.
//
// Prerequisites: build the cluster-agent image and export DD_TEST_CLUSTER_AGENT_IMAGE
// pointing to it (see README.md).
package spot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	kindvmscen "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awskindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	localkubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
)

// Spot scheduling label/annotation constants — mirror pkg/clusteragent/autoscaling/cluster/spot/const.go
// (defined here to avoid a cross-module dependency on a kubeapiserver-tagged package).
const (
	spotEnabledLabelKey         = "autoscaling.datadoghq.com/spot-enabled"
	spotEnabledLabelValue       = "true"
	spotAssignedLabel           = "autoscaling.datadoghq.com/spot-assigned"
	spotConfigAnnotation        = "autoscaling.datadoghq.com/spot-config"
	spotDisabledUntilAnnotation = "autoscaling.datadoghq.com/spot-disabled-until"
	spotCapacityTypeLabel       = "autoscaling.datadoghq.com/capacity-type"
	spotCapacityTypeValue       = "interruptible"

	// kindClusterName is the provisioner name; the kind cluster name is derived from it.
	kindClusterName = "spot-test"
)

// Cluster-agent spot scheduler timeouts used both in Helm values and in test assertions.
// fallbackDuration is 2× scheduleTimeout so the test can observe all pods on-demand
// and uncordon the spot node before the fallback is re-triggered.
const (
	scheduleTimeout              = 30 * time.Second
	fallbackDuration             = 2 * scheduleTimeout
	rebalanceStabilizationPeriod = 10 * time.Second
)

// helmChartVersion is the minimum Datadog Helm chart version that exposes the
// datadog.autoscaling.cluster.spot.enabled feature toggle.
const helmChartVersion = "3.208.0"

// makeHelmValues returns the Helm values for the spot scheduling suite.
// pullPolicy should be "Never" when the image is pre-loaded into kind (local dev)
// or "IfNotPresent" when the image is pulled from a registry (CI).
func makeHelmValues(pullPolicy string) string {
	return fmt.Sprintf(`
clusterAgent:
  enabled: true
  image:
    pullPolicy: %s
  admissionController:
    enabled: true
    mutateUnlabelled: false
  env:
    - name: DD_LOG_LEVEL
      value: "DEBUG"
    - name: DD_AUTOSCALING_CLUSTER_SPOT_DEFAULTS_PERCENTAGE
      value: "100"
    - name: DD_AUTOSCALING_CLUSTER_SPOT_DEFAULTS_MIN_ON_DEMAND_REPLICAS
      value: "0"
    - name: DD_AUTOSCALING_CLUSTER_SPOT_SCHEDULE_TIMEOUT
      value: "%v"
    - name: DD_AUTOSCALING_CLUSTER_SPOT_FALLBACK_DURATION
      value: "%v"
    - name: DD_AUTOSCALING_CLUSTER_SPOT_REBALANCE_STABILIZATION_PERIOD
      value: "%v"
# node-agent DaemonSet not needed; only the cluster-agent runs the spot scheduler
agents:
  enabled: false
# cluster check runners not needed for spot scheduling
clusterChecksRunner:
  enabled: false
datadog:
  kubeStateMetricsCore:
    # the framework sets this to true by default, which unconditionally enables the
    # cluster checks runner deployment regardless of clusterChecksRunner.enabled
    useClusterCheckRunners: false
  autoscaling:
    cluster:
      spot:
        enabled: true
`, pullPolicy, scheduleTimeout, fallbackDuration, rebalanceStabilizationPeriod)
}

// workerNodes defines the kind cluster topology required by the spot scheduling tests:
// one on-demand worker and one spot worker with the interruptible label and taint.
var workerNodes = []kubeComp.KindWorkerNode{
	{}, // on-demand
	{
		Labels: []kubeComp.Label{{Key: "autoscaling.datadoghq.com/capacity-type", Value: "interruptible"}},
		Taints: []kubeComp.Taint{{Key: "autoscaling.datadoghq.com/capacity-type", Value: "interruptible", Effect: "NoSchedule"}},
	},
}

// rebalancingTimeout returns the expected duration to rebalance given number of spot pods.
func rebalancingTimeout(spotPods int) time.Duration {
	return time.Duration(spotPods)*2*rebalanceStabilizationPeriod + 30*time.Second
}

type spotSchedulingSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	kubeClient    k8sclient.Interface
	spotNode      string
	onDemandNode  string
	testNamespace string // namespace for the current test, set by SetupTest
}

// TestSpotSchedulingKind runs spot scheduling integration tests on a local kind cluster.
// Requires DD_TEST_CLUSTER_AGENT_IMAGE to be set to the locally-built cluster-agent image
// (see README.md for build instructions).
func TestSpotSchedulingKind(t *testing.T) {
	image := os.Getenv("DD_TEST_CLUSTER_AGENT_IMAGE")
	if image == "" {
		t.Skip("DD_TEST_CLUSTER_AGENT_IMAGE not set; skipping spot scheduling e2e tests")
	}
	e2e.Run(t, new(spotSchedulingSuite), e2e.WithProvisioner(localkubernetes.Provisioner(
		localkubernetes.WithName(kindClusterName),
		localkubernetes.WithKindWorkerNodes(workerNodes...),
		localkubernetes.WithoutFakeIntake(),
		localkubernetes.WithKindLoadImage(image),
		localkubernetes.WithAgentOptions(
			kubernetesagentparams.WithClusterAgentFullImagePath(image),
			kubernetesagentparams.WithHelmChartVersion(helmChartVersion),
			kubernetesagentparams.WithHelmValues(makeHelmValues("Never")),
		),
	)))
}

// TestSpotSchedulingKindCI runs spot scheduling integration tests on a kind cluster
// provisioned on AWS. The cluster-agent image is resolved automatically from the
// qa_dca ECR image built by this pipeline (cluster-agent-qa:{E2E_PIPELINE_ID}-{E2E_COMMIT_SHA}).
// Requires E2E_PIPELINE_ID to be set (provided by the standard .new_e2e_template CI job).
func TestSpotSchedulingKindCI(t *testing.T) {
	if os.Getenv("E2E_PIPELINE_ID") == "" {
		t.Skip("E2E_PIPELINE_ID not set; this test is for CI use only")
	}
	e2e.Run(t, new(spotSchedulingSuite), e2e.WithProvisioner(awskindvm.Provisioner(
		awskindvm.WithRunOptions(
			kindvmscen.WithName(kindClusterName),
			kindvmscen.WithKindWorkerNodes(workerNodes...),
			kindvmscen.WithoutFakeIntake(),
			kindvmscen.WithAgentOptions(
				kubernetesagentparams.WithHelmChartVersion(helmChartVersion),
				kubernetesagentparams.WithHelmValues(makeHelmValues("IfNotPresent")),
			),
		),
	)))
}

func (s *spotSchedulingSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.kubeClient = s.Env().KubernetesCluster.Client()
	s.identifyNodes()
	s.waitForWebhook()
}

func (s *spotSchedulingSuite) SetupTest() {
	s.createTestNamespace()
}

func (s *spotSchedulingSuite) TearDownTest() {
	s.deleteTestNamespace()
}

// listPods returns active (non-terminating) pods matching selector in s.testNamespace.
// Excludes terminating pods to avoid counting stale entries.
func (s *spotSchedulingSuite) listPods(selector string) []corev1.Pod {
	list, err := s.kubeClient.CoreV1().Pods(s.testNamespace).List(s.T().Context(), metav1.ListOptions{
		LabelSelector: selector,
	})
	s.Require().NoError(err)
	var active []corev1.Pod
	for _, p := range list.Items {
		if p.DeletionTimestamp == nil {
			active = append(active, p)
		}
	}
	return active
}

// eventually retries fn until it passes, with a 1-minute timeout and 5-second polling interval.
func (s *spotSchedulingSuite) eventually(fn func(c *assert.CollectT)) {
	s.EventuallyWithT(fn, 1*time.Minute, 5*time.Second)
}

// expectRunningSpot asserts that exactly count pods are Running on the spot node with spot-assigned label.
func (s *spotSchedulingSuite) expectRunningSpot(c *assert.CollectT, pods []corev1.Pod, count int) {
	actual := 0
	for _, p := range pods {
		if p.Status.Phase == corev1.PodRunning && p.Spec.NodeName == s.spotNode {
			require.Contains(c, p.Labels, spotAssignedLabel, "pod %s on spot node should have spot-assigned label", p.Name)
			actual++
		}
	}
	require.Equal(c, count, actual, "expected %d running spot pods", count)
}

// expectRunningOnDemand asserts that exactly count pods are Running on the on-demand node without spot-assigned label.
func (s *spotSchedulingSuite) expectRunningOnDemand(c *assert.CollectT, pods []corev1.Pod, count int) {
	actual := 0
	for _, p := range pods {
		if p.Status.Phase == corev1.PodRunning && p.Spec.NodeName == s.onDemandNode {
			require.NotContains(c, p.Labels, spotAssignedLabel, "pod %s on on-demand node should not have spot-assigned label", p.Name)
			actual++
		}
	}
	require.Equal(c, count, actual, "expected %d running on-demand pods", count)
}

// identifyNodes finds the spot and on-demand worker nodes by autoscaling.datadoghq.com/capacity-type label.
func (s *spotSchedulingSuite) identifyNodes() {
	s.T().Helper()
	nodes, err := s.kubeClient.CoreV1().Nodes().List(s.T().Context(), metav1.ListOptions{})
	s.Require().NoError(err)
	for _, node := range nodes.Items {
		switch node.Labels[spotCapacityTypeLabel] {
		case spotCapacityTypeValue:
			s.spotNode = node.Name
		default:
			s.onDemandNode = node.Name
		}
	}
	s.Require().NotEmpty(s.spotNode, "no node with %s=%s found; check WithKindWorkerNodes", spotCapacityTypeLabel, spotCapacityTypeValue)
	s.Require().NotEmpty(s.onDemandNode, "no node without %s=%s found; check WithKindWorkerNodes", spotCapacityTypeLabel, spotCapacityTypeValue)
}

// waitForWebhook polls MutatingWebhookConfigurations until the spot scheduling webhook is registered.
func (s *spotSchedulingSuite) waitForWebhook() {
	s.T().Helper()
	s.Require().Eventually(func() bool {
		whList, err := s.kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().List(s.T().Context(), metav1.ListOptions{})
		if err != nil {
			return false
		}
		for _, wh := range whList.Items {
			for _, webhook := range wh.Webhooks {
				if webhook.Name == "datadog.webhook.spot.scheduling" {
					return true
				}
			}
		}
		return false
	}, 5*time.Minute, 5*time.Second, "spot scheduling webhook not registered; is the cluster-agent running?")
}

func (s *spotSchedulingSuite) createTestNamespace() {
	name := s.T().Name()
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	s.testNamespace = strings.ToLower(name)
	if len(s.testNamespace) > 63 {
		s.testNamespace = s.testNamespace[:63]
	}
	_, err := s.kubeClient.CoreV1().Namespaces().Create(s.T().Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: s.testNamespace},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *spotSchedulingSuite) deleteTestNamespace() {
	err := s.kubeClient.CoreV1().Namespaces().Delete(context.Background(), s.testNamespace, metav1.DeleteOptions{})
	s.Require().NoError(err)
}

// cordonNode marks a node as unschedulable and registers an uncordon cleanup.
func (s *spotSchedulingSuite) cordonNode(name string) {
	s.T().Helper()
	_, err := s.kubeClient.CoreV1().Nodes().Patch(s.T().Context(), name,
		types.StrategicMergePatchType,
		[]byte(`{"spec":{"unschedulable":true}}`),
		metav1.PatchOptions{})
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.uncordonNode(name) })
}

// uncordonNode marks a node as schedulable.
func (s *spotSchedulingSuite) uncordonNode(name string) {
	s.T().Helper()
	_, err := s.kubeClient.CoreV1().Nodes().Patch(context.Background(), name,
		types.StrategicMergePatchType,
		[]byte(`{"spec":{"unschedulable":false}}`),
		metav1.PatchOptions{})
	s.Require().NoError(err)
}
