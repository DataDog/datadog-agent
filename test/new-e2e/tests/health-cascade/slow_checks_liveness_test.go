// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package healthcascade tests agent behavior when the Remote Config backend
// becomes unavailable. This simulates the production scenario where RC backend
// outages were observed to cascade into agent liveness failures and pod restarts.
//
// The agent's RC endpoint is pointed at a non-routable IP (192.0.2.1, RFC 5737
// TEST-NET-1). TCP SYN packets to this address are never ACK'd, so the HTTP
// client hangs indefinitely — exactly simulating an RC backend that accepts
// connections but never responds.
package healthcascade

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

// helmValues configures the agent to use a non-routable RC backend.
// 192.0.2.1 is RFC 5737 TEST-NET-1 — TCP SYN packets are never ACK'd,
// so the agent's RC HTTP client hangs indefinitely on connect.
// helmValues points the CLUSTER AGENT's RC at a non-routable IP so its
// RC Fetch call hangs indefinitely. The node agent's RC is left at default.
//
// The hypothesis: cluster agent RC hang → cluster agent degrades →
// node agent endpoint checks timeout waiting for cluster agent →
// workers saturate → scheduler blocks → liveness fails → pod killed.
var helmValues = `
datadog:
  clusterChecks:
    enabled: true
  remoteConfiguration:
    enabled: true
clusterAgent:
  env:
    - name: DD_REMOTE_CONFIGURATION_RC_DD_URL
      value: "http://192.0.2.1:8080"
    - name: DD_REMOTE_CONFIGURATION_NO_TLS
      value: "true"
    - name: DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL
      value: "5s"
`

type rcUnavailableSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestRCBackendUnavailableCausesAgentFailure(t *testing.T) {
	e2e.Run(t, &rcUnavailableSuite{},
		e2e.WithProvisioner(
			provkindvm.Provisioner(
				provkindvm.WithRunOptions(
					scenariokindvm.WithoutFakeIntake(),
					scenariokindvm.WithVMOptions(
						scenec2.WithInstanceType("t3.xlarge"),
					),
					scenariokindvm.WithAgentOptions(
						kubernetesagentparams.WithHelmValues(helmValues),
					),
				),
			),
		),
	)
}

func (s *rcUnavailableSuite) TestRCHangingKillsAgent() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	// ---------------------------------------------------------------
	// Step 1: Find the node agent pod and record its initial state.
	// The agent was deployed with RC pointing at 192.0.2.1 (non-routable),
	// so RC has been hanging since startup.
	// ---------------------------------------------------------------
	s.T().Log("Step 1: finding agent pod and recording initial state")
	agentPod := s.getNodeAgentPod(ctx, k8s)
	s.T().Logf("Found node agent pod: %s", agentPod)

	initialRestarts := s.getAgentRestartCount(ctx, k8s, agentPod)
	s.T().Logf("Initial restart count: %d", initialRestarts)

	// Check initial health.
	stdout, _, err := kc.PodExec("datadog", agentPod, "agent", []string{"agent", "health"})
	if err != nil {
		s.T().Logf("Initial agent health exec error: %v", err)
	} else {
		s.T().Logf("Initial agent health:\n%s", stdout)
	}

	// ---------------------------------------------------------------
	// Step 2: Monitor the agent for 10 minutes.
	//
	// This is the investigation phase. We don't assume what will happen.
	// We log health status and restart counts periodically to discover
	// the failure pattern.
	// ---------------------------------------------------------------
	s.T().Log("Step 2: monitoring agent health while RC backend hangs (10 minutes)")

	unhealthyComponentsSeen := map[string]bool{}
	maxRestarts := initialRestarts
	agentDied := false

	for i := 0; i < 40; i++ { // 40 iterations x 15s = 10 minutes
		time.Sleep(15 * time.Second)

		// Check health.
		stdout, _, err := kc.PodExec("datadog", agentPod, "agent", []string{"agent", "health"})
		if err != nil {
			s.T().Logf("[t+%ds] agent health exec error: %v", (i+1)*15, err)
			agentDied = true
		} else if strings.Contains(stdout, "PASS") {
			s.T().Logf("[t+%ds] Agent health: PASS", (i+1)*15)
		} else {
			s.T().Logf("[t+%ds] Agent health:\n%s", (i+1)*15, stdout)
			for _, line := range strings.Split(stdout, "\n") {
				line = strings.TrimSpace(line)
				if line != "" && !strings.Contains(line, "Agent health") && !strings.Contains(line, "===") {
					unhealthyComponentsSeen[line] = true
				}
			}
		}

		// Check restart count.
		restarts := s.getAgentRestartCount(ctx, k8s, agentPod)
		if restarts > maxRestarts {
			s.T().Logf("[t+%ds] *** AGENT RESTARTED *** count: %d -> %d", (i+1)*15, maxRestarts, restarts)
			maxRestarts = restarts
			agentDied = true
		}

		// If we've confirmed both degradation and restarts, stop early.
		if agentDied && len(unhealthyComponentsSeen) > 0 {
			s.T().Log("Confirmed health degradation + restarts, stopping early")
			break
		}
	}

	// ---------------------------------------------------------------
	// Step 3: Report findings.
	// ---------------------------------------------------------------
	s.T().Log("=== INVESTIGATION RESULTS ===")
	s.T().Logf("Agent restarted: %v (count went from %d to %d)", maxRestarts > initialRestarts, initialRestarts, maxRestarts)
	s.T().Logf("Unhealthy components observed: %v", unhealthyComponentsSeen)

	if agentDied {
		s.T().Log("CONFIRMED: RC backend hanging caused the agent to die/restart")
	} else {
		s.T().Log("NOT CONFIRMED: Agent survived 10 minutes with hanging RC backend")
	}

	// Capture final agent status for diagnostics.
	stdout, _, _ = kc.PodExec("datadog", agentPod, "agent", []string{"agent", "status"})
	if len(stdout) > 2000 {
		stdout = stdout[:2000] + "\n... (truncated)"
	}
	s.T().Logf("Agent status:\n%s", stdout)

	// The test PASSES if the agent dies — confirming the production issue.
	// The test FAILS if the agent survives — meaning RC unavailability alone
	// doesn't kill the agent and the production issue has additional causes.
	assert.True(s.T(), agentDied,
		"Expected agent to die/restart when RC backend hangs, but it survived. "+
			"The production issue may require additional conditions beyond RC unavailability alone.")
}

// getNodeAgentPod returns the name of the running node agent pod (not cluster agent, not CSI driver).
func (s *rcUnavailableSuite) getNodeAgentPod(ctx context.Context, k8s kubeClient.Interface) string {
	s.T().Helper()
	var agentPod string
	require.Eventually(s.T(), func() bool {
		pods, err := k8s.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: "app=dda-linux-datadog",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				agentPod = p.Name
				return true
			}
		}
		return false
	}, 3*time.Minute, 5*time.Second, "should find a running node agent pod")
	return agentPod
}

// getAgentRestartCount returns the restart count for the agent container.
func (s *rcUnavailableSuite) getAgentRestartCount(ctx context.Context, k8s kubeClient.Interface, podName string) int32 {
	s.T().Helper()
	pod, err := k8s.CoreV1().Pods("datadog").Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return 0
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "agent" {
			return cs.RestartCount
		}
	}
	return 0
}
