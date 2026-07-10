// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

//go:embed fixtures/admission_probe_helm_values.yaml
var admissionProbeHelmValues string

const (
	admissionProbeIssueID = "admission-controller-connectivity-failure"
	admissionWebhookPort  = "8000"
	disruptorPodName      = "network-disruptor"
	disruptorNamespace    = "default"
	clusterAgentNamespace = "datadog"
)

type admissionProbeSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestAdmissionProbeSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &admissionProbeSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(admissionProbeHelmValues),
				),
			),
		)),
	)
}

// TestAdmissionProbeIssueLifecycle tests the full lifecycle:
//  1. Probe is healthy (no issue)
//  2. Block port 8000 with iptables → probe fails → health issue reported to fakeintake
//  3. Unblock port → probe succeeds → health issue absent from subsequent reports
func (suite *admissionProbeSuite) TestAdmissionProbeIssueLifecycle() {
	ctx := context.Background()
	fakeIntake := suite.Env().FakeIntake.Client()

	clusterAgentPod := suite.findClusterAgentPod(ctx)
	nodeName := clusterAgentPod.Spec.NodeName
	suite.T().Logf("Cluster agent pod %s is on node %s", clusterAgentPod.Name, nodeName)

	suite.deployDisruptorPod(ctx, nodeName)
	defer suite.cleanupDisruptorPod(ctx)

	// =========================================================================
	// Phase 1: Verify probe is healthy
	// =========================================================================
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		stdout, _, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
			clusterAgentNamespace, clusterAgentPod.Name, "cluster-agent",
			[]string{"env", "DD_LOG_LEVEL=off", "datadog-cluster-agent", "status", "--json"},
		)
		assert.NoError(ct, err)

		var status map[string]any
		assert.NoError(ct, json.Unmarshal([]byte(stdout), &status))

		webhook, ok := status["admissionWebhook"].(map[string]any)
		if !assert.True(ct, ok, "admissionWebhook section not found in status") {
			return
		}
		probeSection, ok := webhook["Probe"].(map[string]any)
		if !assert.True(ct, ok, "Probe section not found in admissionWebhook status") {
			return
		}
		assert.Equal(ct, true, probeSection["LastExecutionSuccess"])
	}, 3*time.Minute, 15*time.Second, "Probe has not completed a successful execution")

	suite.T().Run("IssueDetection", func(t *testing.T) {
		suite.blockWebhookFromAPIServer(t)

		require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

		var detectedIssue *healthplatform.Issue
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, admissionProbeIssueID) {
					if iss.PersistedIssue != nil &&
						(iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE) {
						detectedIssue = iss
						return
					}
				}
			}
			assert.Fail(ct, "admission probe issue not found as ACTIVE in fakeintake")
		}, defaultIssueTimeout, defaultIssuePollInterval, "admission probe issue not detected in fakeintake")

		require.NotNil(t, detectedIssue)
		assert.Equal(t, admissionProbeIssueID, detectedIssue.Id)
		assert.Equal(t, "availability", detectedIssue.Category)
		assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, detectedIssue.Severity)
		assert.Equal(t, "cluster-agent", detectedIssue.Source)
		assert.NotNil(t, detectedIssue.Remediation)
		assert.NotEmpty(t, detectedIssue.Remediation.Steps)
		require.NotNil(t, detectedIssue.PersistedIssue)
		assert.Equal(t, healthplatform.IssueState_ISSUE_STATE_ACTIVE, detectedIssue.PersistedIssue.State)
	})

	suite.unblockWebhookFromAPIServer(suite.T())

	suite.T().Run("Resolution", func(t *testing.T) {
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			assert.NoError(ct, err)
			for _, p := range payloads {
				for _, iss := range findIssuesByID(t, p, admissionProbeIssueID) {
					if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
						return
					}
				}
			}
			assert.Fail(ct, "no payload found with the issue in RESOLVED state")
		}, defaultIssueTimeout, defaultIssuePollInterval, "admission probe issue never transitioned to RESOLVED")
	})

}

// ============================================================================
// Helpers
// ============================================================================

func (suite *admissionProbeSuite) findClusterAgentPod(ctx context.Context) corev1.Pod {
	client := suite.Env().KubernetesCluster.Client()

	var clusterAgentPod corev1.Pod
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		pods, err := client.CoreV1().Pods(clusterAgentNamespace).List(ctx, metav1.ListOptions{})
		assert.NoError(ct, err)
		for _, pod := range pods.Items {
			if strings.Contains(pod.Name, "cluster-agent") && pod.Status.Phase == corev1.PodRunning {
				clusterAgentPod = pod
				return
			}
		}
		assert.Fail(ct, "Cluster agent pod not found or not running")
	}, 3*time.Minute, 10*time.Second, "Cluster agent pod not found")

	return clusterAgentPod
}

func (suite *admissionProbeSuite) deployDisruptorPod(ctx context.Context, nodeName string) {
	client := suite.Env().KubernetesCluster.Client()
	privileged := true
	hostPID := true

	// Get the agent DaemonSet image (guaranteed to be already pulled in the cluster)
	ds, err := client.AppsV1().DaemonSets("datadog").Get(ctx, "dda-linux-datadog", metav1.GetOptions{})
	require.NoError(suite.T(), err, "Failed to get agent DaemonSet")
	agentImage := ds.Spec.Template.Spec.Containers[0].Image

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      disruptorPodName,
			Namespace: disruptorNamespace,
		},
		Spec: corev1.PodSpec{
			NodeName:    nodeName,
			HostPID:     hostPID,
			HostNetwork: true,
			Containers: []corev1.Container{
				{
					Name:    "disruptor",
					Image:   agentImage,
					Command: []string{"sleep", "infinity"},
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{"NET_ADMIN", "SYS_PTRACE", "SYS_ADMIN"},
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			Tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists},
			},
		},
	}

	// Delete any leftover disruptor pod from a previous run
	_ = client.CoreV1().Pods(disruptorNamespace).Delete(ctx, disruptorPodName, metav1.DeleteOptions{})
	time.Sleep(5 * time.Second)

	_, err = client.CoreV1().Pods(disruptorNamespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(suite.T(), err, "Failed to create disruptor pod")

	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		p, err := client.CoreV1().Pods(disruptorNamespace).Get(ctx, disruptorPodName, metav1.GetOptions{})
		assert.NoError(ct, err)
		assert.Equal(ct, corev1.PodRunning, p.Status.Phase)
	}, 2*time.Minute, 5*time.Second, "Disruptor pod not running")

	// The agent image is Debian-based and has iptables pre-installed via the node's
	// mount namespace (hostPID gives access). Verify iptables is accessible via nsenter.
	_, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		disruptorNamespace, disruptorPodName, "disruptor",
		[]string{"sh", "-c", "which iptables || nsenter -t 1 -m -- which iptables"},
	)
	require.NoError(suite.T(), err, "iptables not available: %s", stderr)
}

func (suite *admissionProbeSuite) cleanupDisruptorPod(ctx context.Context) {
	client := suite.Env().KubernetesCluster.Client()
	// Best-effort: remove any iptables rule left behind
	_, _, _ = suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		disruptorNamespace, disruptorPodName, "disruptor",
		[]string{"sh", "-c", "IPTABLES=$(which iptables 2>/dev/null || echo 'nsenter -t 1 -m -- iptables'); $IPTABLES -D OUTPUT -p tcp --dport 8000 -j DROP 2>/dev/null; true"},
	)
	_ = client.CoreV1().Pods(disruptorNamespace).Delete(ctx, disruptorPodName, metav1.DeleteOptions{})
}

// blockWebhookFromAPIServer applies an iptables OUTPUT rule in the node's network namespace
// (shared by the disruptor pod via hostNetwork and by the API server via hostNetwork).
// This prevents the API server from making outbound connections to the webhook on port 8000.
// Uses nsenter to access the node's iptables binary if not available directly.
func (suite *admissionProbeSuite) blockWebhookFromAPIServer(t *testing.T) {
	t.Helper()
	cmd := fmt.Sprintf(
		"IPTABLES=$(which iptables 2>/dev/null || echo 'nsenter -t 1 -m -- iptables') && "+
			"$IPTABLES -I OUTPUT -p tcp --dport %s -j DROP",
		admissionWebhookPort,
	)
	_, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		disruptorNamespace, disruptorPodName, "disruptor",
		[]string{"sh", "-c", cmd},
	)
	require.NoError(t, err, "Failed to block webhook port: stderr=%s", stderr)
}

// unblockWebhookFromAPIServer removes the iptables OUTPUT rule from the node's network namespace.
func (suite *admissionProbeSuite) unblockWebhookFromAPIServer(t *testing.T) {
	t.Helper()
	cmd := fmt.Sprintf(
		"IPTABLES=$(which iptables 2>/dev/null || echo 'nsenter -t 1 -m -- iptables') && "+
			"$IPTABLES -D OUTPUT -p tcp --dport %s -j DROP",
		admissionWebhookPort,
	)
	_, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		disruptorNamespace, disruptorPodName, "disruptor",
		[]string{"sh", "-c", cmd},
	)
	require.NoError(t, err, "Failed to unblock webhook port: stderr=%s", stderr)
}
