// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package healthcascade tests agent behavior when the Remote Config backend
// becomes unavailable. This simulates the production scenario where RC backend
// outages were observed to cascade into agent liveness failures and pod restarts.
//
// The test deploys a fake RC backend server inside the Kind cluster that hangs
// for 2 minutes on every request, then points the agent at it. We observe
// whether the agent stays healthy or degrades/dies, and capture exactly which
// components fail and when.
package healthcascade

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeClient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

const (
	fakeRCNs   = "default"
	fakeRCName = "fake-rc-backend"
	fakeRCPort = 8080
	// How long the fake RC server delays before responding.
	// Simulates an RC backend that accepts connections but hangs.
	fakeRCDelaySecs = 120
)

// helmValues configures the agent to use our fake RC backend.
//   - remote_configuration.rc_dd_url: points to the fake server Service inside the cluster
//   - remote_configuration.no_tls: allows plain HTTP (fake server doesn't do TLS)
//   - remote_configuration.no_tls_validation: skip cert checks
//   - remote_configuration.refresh_interval: poll frequently so we see the effect faster
var helmValues = fmt.Sprintf(`
datadog:
  remoteConfiguration:
    enabled: true
agents:
  containers:
    agent:
      env:
        - name: DD_REMOTE_CONFIGURATION_RC_DD_URL
          value: "http://%s.%s.svc.cluster.local:%d"
        - name: DD_REMOTE_CONFIGURATION_NO_TLS
          value: "true"
        - name: DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL
          value: "5s"
`, fakeRCName, fakeRCNs, fakeRCPort)

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
	// Step 1: Deploy the fake RC backend that hangs on every request.
	//
	// This is a Python HTTP server that accepts connections but sleeps
	// for 2 minutes before responding — simulating the RC backend
	// being unreachable/timing out, which is the exact production
	// scenario we're investigating.
	// ---------------------------------------------------------------
	s.T().Log("Step 1: deploying fake hanging RC backend")
	s.deployFakeRCBackend(ctx, k8s)

	// Wait for fake RC to be running.
	s.EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods(fakeRCNs).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", fakeRCName),
		})
		assert.NoError(c, err)
		for _, p := range pods.Items {
			for _, cond := range p.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return
				}
			}
		}
		assert.Fail(c, "fake RC backend pod not ready yet")
	}, 2*time.Minute, 5*time.Second, "fake RC backend should be running")

	// ---------------------------------------------------------------
	// Step 2: Find the agent pod and record its initial state.
	// ---------------------------------------------------------------
	s.T().Log("Step 2: finding agent pod and recording initial state")
	agentPod := s.getAgentPod(ctx, k8s)
	s.T().Logf("Found agent pod: %s", agentPod)

	initialRestarts := s.getAgentRestartCount(ctx, k8s, agentPod)
	s.T().Logf("Initial restart count: %d", initialRestarts)

	// Check initial health — it may or may not be healthy depending on
	// whether the agent has already tried to reach the fake RC backend.
	stdout, _, _ := kc.PodExec("datadog", agentPod, "agent", []string{"agent", "health"})
	s.T().Logf("Initial agent health:\n%s", stdout)

	// ---------------------------------------------------------------
	// Step 3: Monitor the agent for 10 minutes.
	//
	// This is the investigation phase. We don't assume what will happen.
	// We log health status and restart counts periodically to discover
	// the failure pattern: does the agent die? Which components fail?
	// How long does it take?
	// ---------------------------------------------------------------
	s.T().Log("Step 3: monitoring agent health while RC backend hangs")

	unhealthyComponentsSeen := map[string]bool{}
	maxRestarts := initialRestarts
	agentDied := false

	for i := 0; i < 40; i++ { // 40 iterations x 15s = 10 minutes
		time.Sleep(15 * time.Second)

		// Check health.
		stdout, _, err := kc.PodExec("datadog", agentPod, "agent", []string{"agent", "health"})
		if err != nil {
			s.T().Logf("[t+%ds] agent health exec error (agent may be restarting): %v", (i+1)*15, err)
			agentDied = true
		} else if strings.Contains(stdout, "PASS") {
			s.T().Logf("[t+%ds] Agent health: PASS", (i+1)*15)
		} else {
			s.T().Logf("[t+%ds] Agent health:\n%s", (i+1)*15, stdout)
			// Parse out unhealthy component names.
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
			s.T().Logf("[t+%ds] *** AGENT RESTARTED *** restart count: %d -> %d", (i+1)*15, maxRestarts, restarts)
			maxRestarts = restarts
			agentDied = true
		}

		// If we've seen both health failure and restarts, we have our answer.
		if agentDied && len(unhealthyComponentsSeen) > 0 {
			s.T().Log("Observed both health degradation and pod restarts — collecting final diagnostics")
			break
		}
	}

	// ---------------------------------------------------------------
	// Step 4: Report findings.
	// ---------------------------------------------------------------
	s.T().Log("=== INVESTIGATION RESULTS ===")
	s.T().Logf("Agent restarted: %v (restart count went from %d to %d)", maxRestarts > initialRestarts, initialRestarts, maxRestarts)
	s.T().Logf("Unhealthy components observed: %v", unhealthyComponentsSeen)

	if agentDied {
		s.T().Logf("CONFIRMED: RC backend hanging caused the agent to die/restart")
	} else {
		s.T().Logf("NOT CONFIRMED: Agent survived 10 minutes with hanging RC backend")
	}

	// Try to get agent logs for the smoking gun.
	agentLogs, _, _ := kc.PodExec("datadog", agentPod, "agent", []string{
		"agent", "status",
	})
	if len(agentLogs) > 2000 {
		agentLogs = agentLogs[:2000] + "\n... (truncated)"
	}
	s.T().Logf("Agent status (truncated):\n%s", agentLogs)

	// The test PASSES if we observe agent death — confirming the production issue.
	// The test FAILS if the agent survives — meaning the production issue has
	// a different root cause than RC backend unavailability alone.
	assert.True(s.T(), agentDied,
		"Expected agent to die/restart when RC backend is hanging, but it survived. "+
			"The production issue may require additional conditions beyond just RC unavailability.")
}

// getAgentPod returns the name of a running node agent pod.
func (s *rcUnavailableSuite) getAgentPod(ctx context.Context, k8s kubeClient.Interface) string {
	s.T().Helper()
	var agentPod string
	require.Eventually(s.T(), func() bool {
		pods, err := k8s.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, p := range pods.Items {
			if strings.Contains(p.Name, "cluster-agent") {
				continue
			}
			if strings.Contains(p.Name, "cluster-checks") {
				continue
			}
			if !strings.Contains(p.Name, "datadog") {
				continue
			}
			if p.Status.Phase == corev1.PodRunning {
				agentPod = p.Name
				return true
			}
		}
		return false
	}, 3*time.Minute, 5*time.Second, "should find a running agent pod")
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

// deployFakeRCBackend creates a Deployment + Service for a fake RC backend
// that hangs for 2 minutes on every request.
func (s *rcUnavailableSuite) deployFakeRCBackend(ctx context.Context, k8s kubeClient.Interface) {
	s.T().Helper()
	replicas := int32(1)

	// Python server that accepts connections but sleeps before responding.
	// This simulates the RC backend being slow/unreachable.
	pythonServer := fmt.Sprintf(`
from http.server import HTTPServer, BaseHTTPRequestHandler
import time
class H(BaseHTTPRequestHandler):
    def do_POST(self):
        time.sleep(%d)
        self.send_response(503)
        self.end_headers()
        self.wfile.write(b'Service Unavailable')
    def do_GET(self):
        time.sleep(%d)
        self.send_response(503)
        self.end_headers()
        self.wfile.write(b'Service Unavailable')
    def log_message(self, *a): pass
HTTPServer(('', %d), H).serve_forever()
`, fakeRCDelaySecs, fakeRCDelaySecs, fakeRCPort)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeRCName,
			Namespace: fakeRCNs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": fakeRCName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": fakeRCName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "fake-rc",
							Image:   "python:3-slim",
							Command: []string{"python3", "-c", pythonServer},
							Ports: []corev1.ContainerPort{
								{ContainerPort: int32(fakeRCPort)},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(fakeRCPort),
									},
								},
								InitialDelaySeconds: 3,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeRCName,
			Namespace: fakeRCNs,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": fakeRCName},
			Ports: []corev1.ServicePort{
				{
					Port:       int32(fakeRCPort),
					TargetPort: intstr.FromInt(fakeRCPort),
				},
			},
		},
	}

	_, err := k8s.AppsV1().Deployments(fakeRCNs).Create(ctx, deploy, metav1.CreateOptions{})
	require.NoError(s.T(), err, "failed to create fake RC backend deployment")

	_, err = k8s.CoreV1().Services(fakeRCNs).Create(ctx, svc, metav1.CreateOptions{})
	require.NoError(s.T(), err, "failed to create fake RC backend service")
}
