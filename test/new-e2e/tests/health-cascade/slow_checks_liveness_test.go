// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package healthcascade tests agent liveness when the RC backend is unavailable.
//
// Production data from the Apr 13 outage shows:
//   - Agent health failures correlated DIRECTLY with RC recovery
//   - Slow checks (~220K/hour) were constant BEFORE, DURING, and AFTER the outage
//   - When RC recovered (19:00-20:00), health failures stopped, even though
//     slow checks INCREASED (agents alive = more check activity)
//
// This means RC unavailability is the trigger, NOT slow checks. But our
// isolated e2e tests with hanging RC alone don't reproduce it — the
// mechanism requires additional conditions present in a real staging
// environment (many workloads, many checks, cluster agent interactions, etc).
//
// These tests systematically vary conditions to find the trigger.
// Tests PASS when the agent stays healthy (desired behavior).
// Tests FAIL when the agent dies (indicates a bug to fix).
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
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeClient "k8s.io/client-go/kubernetes"

	e2eclient "github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func kindProvisioner(helmValues string) e2e.SuiteOption {
	return e2e.WithProvisioner(
		provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenariokindvm.WithoutFakeIntake(),
				scenariokindvm.WithVMOptions(scenec2.WithInstanceType("t3.xlarge")),
				scenariokindvm.WithDeployTestWorkload(),
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(helmValues),
				),
			),
		),
	)
}

func getNodeAgentPod(ctx context.Context, t *testing.T, k8s kubeClient.Interface) string {
	t.Helper()
	var pod string
	require.Eventually(t, func() bool {
		pods, err := k8s.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: "app=dda-linux-datadog",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				pod = p.Name
				return true
			}
		}
		return false
	}, 3*time.Minute, 5*time.Second, "should find a running node agent pod")
	return pod
}

func getRestartCount(ctx context.Context, t *testing.T, k8s kubeClient.Interface, podName string) int32 {
	t.Helper()
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

type monitorResult struct {
	Restarts            int32
	UnhealthyComponents map[string]bool
	AgentDied           bool
	HealthPassCount     int
	HealthFailCount     int
	ExecErrorCount      int
}

func monitorAgent(ctx context.Context, t *testing.T, kc *e2eclient.KubernetesClient, k8s kubeClient.Interface, podName string, duration time.Duration, interval time.Duration) monitorResult {
	t.Helper()
	result := monitorResult{
		UnhealthyComponents: make(map[string]bool),
	}
	initialRestarts := getRestartCount(ctx, t, k8s, podName)
	iterations := int(duration / interval)

	// Skip the first few checks to allow the agent to finish startup.
	// The agent may not be fully ready for the first ~30 seconds.
	startedHealthy := false

	for i := range iterations {
		time.Sleep(interval)
		elapsed := time.Duration(i+1) * interval

		stdout, _, err := kc.PodExec("datadog", podName, "agent", []string{"agent", "health"})
		if err != nil {
			if startedHealthy {
				t.Logf("[t+%s] exec error (after being healthy): %v", elapsed, err)
				result.ExecErrorCount++
				result.AgentDied = true
			} else {
				t.Logf("[t+%s] exec error (agent still starting): %v", elapsed, err)
			}
		} else if strings.Contains(stdout, "PASS") {
			startedHealthy = true
			t.Logf("[t+%s] Agent health: PASS", elapsed)
			result.HealthPassCount++
		} else {
			if startedHealthy {
				t.Logf("[t+%s] Agent health FAILED:\n%s", elapsed, stdout)
				result.HealthFailCount++
				for _, line := range strings.Split(stdout, "\n") {
					line = strings.TrimSpace(line)
					if line != "" && !strings.Contains(line, "Agent health") && !strings.Contains(line, "===") {
						result.UnhealthyComponents[line] = true
					}
				}
			} else {
				t.Logf("[t+%s] Agent health (still starting):\n%s", elapsed, stdout)
			}
		}

		restarts := getRestartCount(ctx, t, k8s, podName)
		if restarts > initialRestarts && startedHealthy {
			t.Logf("[t+%s] *** AGENT RESTARTED *** count: %d -> %d", elapsed, initialRestarts, restarts)
			result.AgentDied = true
		}
		result.Restarts = restarts - initialRestarts

		if result.AgentDied && len(result.UnhealthyComponents) > 0 {
			t.Log("Observed health degradation and agent death — stopping early")
			break
		}
	}
	return result
}

func reportResults(t *testing.T, result monitorResult) {
	t.Helper()
	t.Log("=== RESULTS ===")
	t.Logf("Agent died/restarted: %v (restarts: %d)", result.AgentDied, result.Restarts)
	t.Logf("Health PASS: %d, FAIL: %d, exec errors: %d", result.HealthPassCount, result.HealthFailCount, result.ExecErrorCount)
	if len(result.UnhealthyComponents) > 0 {
		t.Logf("Unhealthy components: %v", result.UnhealthyComponents)
	}
}

// ---------------------------------------------------------------------------
// Test 1: RC backend hanging (cluster agent side)
//
// Tests that the agent stays healthy when only the cluster agent's
// RC endpoint is unavailable. Minimal Kind environment.
// ---------------------------------------------------------------------------

type rcHangingSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestRCBackendHanging(t *testing.T) {
	e2e.Run(t, &rcHangingSuite{}, kindProvisioner(`
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
`))
}

func (s *rcHangingSuite) TestAgentSurvivesRCHanging() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 5*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy when only the cluster agent's RC backend is hanging")
}

// ---------------------------------------------------------------------------
// Test 2: Cluster agent DOWN (scaled to 0)
//
// Tests that the agent stays healthy when the cluster agent is
// completely unavailable.
// ---------------------------------------------------------------------------

type clusterAgentDownSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestClusterAgentDown(t *testing.T) {
	e2e.Run(t, &clusterAgentDownSuite{}, kindProvisioner(`
datadog:
  clusterChecks:
    enabled: true
`))
}

func (s *clusterAgentDownSuite) TestAgentSurvivesClusterAgentDown() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec("datadog", pod, "agent", []string{"agent", "health"})
		assert.NoError(c, err)
		assert.Contains(c, stdout, "Agent health: PASS")
	}, 3*time.Minute, 10*time.Second, "agent should be healthy before fault injection")

	s.T().Log("Scaling cluster agent to 0 replicas")
	zero := int32(0)
	_, err := k8s.AppsV1().Deployments("datadog").UpdateScale(ctx,
		"dda-linux-datadog-cluster-agent",
		&autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{Name: "dda-linux-datadog-cluster-agent", Namespace: "datadog"},
			Spec:       autoscalingv1.ScaleSpec{Replicas: zero},
		},
		metav1.UpdateOptions{})
	require.NoError(s.T(), err)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: "app=dda-linux-datadog-cluster-agent",
		})
		assert.NoError(c, err)
		assert.Empty(c, pods.Items)
	}, 2*time.Minute, 5*time.Second, "cluster agent should be scaled to 0")

	s.T().Log("Monitoring node agent for 10 minutes with cluster agent down")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 10*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy when the cluster agent is down")
}

// ---------------------------------------------------------------------------
// Test 3: Kitchen sink — RC hanging in a realistic environment
//
// Deploys a more realistic environment with test workloads, then makes
// RC unavailable for BOTH the node agent and cluster agent. This is
// closer to the production scenario where RC was the correlated variable.
//
// The key difference from Test 1: this test uses WithDeployTestWorkload()
// to simulate a busier environment with actual autodiscovered checks.
// ---------------------------------------------------------------------------

type kitchenSinkSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestKitchenSinkRCHanging(t *testing.T) {
	e2e.Run(t, &kitchenSinkSuite{}, kindProvisioner(`
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
agents:
  containers:
    agent:
      env:
        - name: DD_REMOTE_CONFIGURATION_RC_DD_URL
          value: "http://192.0.2.1:8080"
        - name: DD_REMOTE_CONFIGURATION_NO_TLS
          value: "true"
        - name: DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL
          value: "5s"
`))
}

func (s *kitchenSinkSuite) TestAgentSurvivesRCHangingWithWorkloads() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	s.T().Log("Monitoring for 10 min with RC hanging for both agents + test workloads running")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 10*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy when RC is hanging, even with active workloads")
}

// ---------------------------------------------------------------------------
// Test 4: Many slow checks (scheduler blocking mechanism)
//
// This tests the specific scheduler blocking bug independently of RC.
// Deploys pods with HTTP checks that take 20 seconds each, with only
// 4 workers. This SHOULD trigger liveness failure via the unbuffered
// pendingChecksChan blocking mechanism.
//
// This test PASSES if the agent stays healthy (meaning the slow checks
// don't actually trigger the bug in practice). It FAILS if the agent
// dies (confirming the bug is reachable with enough slow checks).
// ---------------------------------------------------------------------------

type slowChecksSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestSlowChecksSchedulerBlocking(t *testing.T) {
	e2e.Run(t, &slowChecksSuite{}, kindProvisioner(`
datadog:
  clusterChecks:
    enabled: true
agents:
  containers:
    agent:
      env:
        - name: DD_CHECK_RUNNERS
          value: "4"
`))
}

func (s *slowChecksSuite) TestAgentWithManySlowChecks() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec("datadog", pod, "agent", []string{"agent", "health"})
		assert.NoError(c, err)
		assert.Contains(c, stdout, "Agent health: PASS")
	}, 3*time.Minute, 10*time.Second, "agent should be healthy before deploying slow checks")

	s.T().Log("Deploying 15 slow HTTP server pods (20s delay, 4 workers)")
	s.deploySlowServer(ctx, k8s)
	defer s.cleanupSlowServer(ctx, k8s)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
			LabelSelector: "app=slow-http-server",
		})
		assert.NoError(c, err)
		ready := 0
		for _, p := range pods.Items {
			for _, cond := range p.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					ready++
				}
			}
		}
		assert.GreaterOrEqual(c, ready, 15)
	}, 3*time.Minute, 10*time.Second, "slow server pods should be running")

	s.T().Log("Monitoring agent for 10 minutes with slow checks active")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 10*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	// This tests the scheduler blocking mechanism. If the agent dies,
	// we've confirmed the bug is reachable in practice.
	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy even with many slow checks. "+
			"If this fails, slow checks triggered the scheduler blocking bug.")
}

func (s *slowChecksSuite) deploySlowServer(ctx context.Context, k8s kubeClient.Interface) {
	s.T().Helper()
	replicas := int32(15)

	pythonServer := fmt.Sprintf(`
from http.server import HTTPServer, BaseHTTPRequestHandler
import time
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        time.sleep(%d)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'ok')
    def log_message(self, *a): pass
HTTPServer(('', %d), H).serve_forever()
`, 20, 8080)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "slow-http-server", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "slow-http-server"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "slow-http-server"},
					Annotations: map[string]string{
						"ad.datadoghq.com/slow-server.check_names":  `["http_check"]`,
						"ad.datadoghq.com/slow-server.init_configs": `[{}]`,
						"ad.datadoghq.com/slow-server.instances":    `[{"name":"slow_endpoint","url":"http://%%host%%:8080/","timeout":25,"min_collection_interval":15}]`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "slow-server",
						Image:   "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/python:3-slim",
						Command: []string{"python3", "-c", pythonServer},
						Ports:   []corev1.ContainerPort{{ContainerPort: 8080}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(8080)},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
						},
					}},
				},
			},
		},
	}

	_, err := k8s.AppsV1().Deployments("default").Create(ctx, deploy, metav1.CreateOptions{})
	require.NoError(s.T(), err)
}

func (s *slowChecksSuite) cleanupSlowServer(ctx context.Context, k8s kubeClient.Interface) {
	s.T().Helper()
	_ = k8s.AppsV1().Deployments("default").Delete(ctx, "slow-http-server", metav1.DeleteOptions{})
	_ = assert.Eventually(s.T(), func() bool {
		pods, err := k8s.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
			LabelSelector: "app=slow-http-server",
		})
		return err == nil && len(pods.Items) == 0
	}, 2*time.Minute, 5*time.Second)
}

// ---------------------------------------------------------------------------
// Test 5: RC hanging + slow checks — THE PRODUCTION SCENARIO
//
// This is the combination the production data points to:
//   - RC unavailable for both node agent and cluster agent
//   - Many checks running slowly (constant ~220K/hour watchdog warnings)
//   - Agent health failures correlated with RC, not check performance
//
// If RC hanging alone doesn't kill the agent (Test 1, Test 3),
// and slow checks alone don't kill the agent (Test 4),
// does the COMBINATION kill it?
// ---------------------------------------------------------------------------

type rcPlusSlowChecksSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestRCHangingPlusSlowChecks(t *testing.T) {
	e2e.Run(t, &rcPlusSlowChecksSuite{}, kindProvisioner(`
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
agents:
  containers:
    agent:
      env:
        - name: DD_REMOTE_CONFIGURATION_RC_DD_URL
          value: "http://192.0.2.1:8080"
        - name: DD_REMOTE_CONFIGURATION_NO_TLS
          value: "true"
        - name: DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL
          value: "5s"
        - name: DD_CHECK_RUNNERS
          value: "4"
`))
}

func (s *rcPlusSlowChecksSuite) TestRCHangingWithSlowChecks() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec("datadog", pod, "agent", []string{"agent", "health"})
		assert.NoError(c, err)
		assert.Contains(c, stdout, "Agent health: PASS")
	}, 3*time.Minute, 10*time.Second, "agent should be healthy before deploying slow checks")

	// Deploy slow checks while RC is already hanging.
	s.T().Log("Deploying 15 slow HTTP server pods (RC already hanging for both agents)")
	s.deploySlowServer(ctx, k8s)
	defer s.cleanupSlowServer(ctx, k8s)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
			LabelSelector: "app=slow-http-server",
		})
		assert.NoError(c, err)
		ready := 0
		for _, p := range pods.Items {
			for _, cond := range p.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					ready++
				}
			}
		}
		assert.GreaterOrEqual(c, ready, 15)
	}, 3*time.Minute, 10*time.Second, "slow server pods should be running")

	s.T().Log("Monitoring for 10 min: RC hanging + slow checks active")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 10*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy with RC hanging + slow checks. "+
			"If this fails, the combination of RC unavailability and slow checks is the trigger.")
}

func (s *rcPlusSlowChecksSuite) deploySlowServer(ctx context.Context, k8s kubeClient.Interface) {
	s.T().Helper()
	replicas := int32(15)

	pythonServer := fmt.Sprintf(`
from http.server import HTTPServer, BaseHTTPRequestHandler
import time
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        time.sleep(%d)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'ok')
    def log_message(self, *a): pass
HTTPServer(('', %d), H).serve_forever()
`, 20, 8080)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "slow-http-server", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "slow-http-server"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "slow-http-server"},
					Annotations: map[string]string{
						"ad.datadoghq.com/slow-server.check_names":  `["http_check"]`,
						"ad.datadoghq.com/slow-server.init_configs": `[{}]`,
						"ad.datadoghq.com/slow-server.instances":    `[{"name":"slow_endpoint","url":"http://%%host%%:8080/","timeout":25,"min_collection_interval":15}]`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "slow-server",
						Image:   "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/python:3-slim",
						Command: []string{"python3", "-c", pythonServer},
						Ports:   []corev1.ContainerPort{{ContainerPort: 8080}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(8080)},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
						},
					}},
				},
			},
		},
	}

	_, err := k8s.AppsV1().Deployments("default").Create(ctx, deploy, metav1.CreateOptions{})
	require.NoError(s.T(), err)
}

func (s *rcPlusSlowChecksSuite) cleanupSlowServer(ctx context.Context, k8s kubeClient.Interface) {
	s.T().Helper()
	_ = k8s.AppsV1().Deployments("default").Delete(ctx, "slow-http-server", metav1.DeleteOptions{})
	_ = assert.Eventually(s.T(), func() bool {
		pods, err := k8s.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
			LabelSelector: "app=slow-http-server",
		})
		return err == nil && len(pods.Items) == 0
	}, 2*time.Minute, 5*time.Second)
}

// ---------------------------------------------------------------------------
// Test 6: RC hanging + cluster agent down + workloads
//
// Combined failure: RC unavailable for both agents, cluster agent killed,
// and test workloads running. The most aggressive scenario.
// ---------------------------------------------------------------------------

type combinedFailureSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestCombinedRCAndClusterAgentDown(t *testing.T) {
	e2e.Run(t, &combinedFailureSuite{}, kindProvisioner(`
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
agents:
  containers:
    agent:
      env:
        - name: DD_REMOTE_CONFIGURATION_RC_DD_URL
          value: "http://192.0.2.1:8080"
        - name: DD_REMOTE_CONFIGURATION_NO_TLS
          value: "true"
`))
}

func (s *combinedFailureSuite) TestAgentSurvivesCombinedFailure() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec("datadog", pod, "agent", []string{"agent", "health"})
		assert.NoError(c, err)
		assert.Contains(c, stdout, "Agent health: PASS")
	}, 3*time.Minute, 10*time.Second, "agent should be healthy before fault injection")

	s.T().Log("Scaling cluster agent to 0 (RC already hanging)")
	zero := int32(0)
	_, err := k8s.AppsV1().Deployments("datadog").UpdateScale(ctx,
		"dda-linux-datadog-cluster-agent",
		&autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{Name: "dda-linux-datadog-cluster-agent", Namespace: "datadog"},
			Spec:       autoscalingv1.ScaleSpec{Replicas: zero},
		},
		metav1.UpdateOptions{})
	require.NoError(s.T(), err)

	s.T().Log("Monitoring for 10 min: RC hanging + cluster agent down + workloads")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 10*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy even with RC hanging, cluster agent down, and workloads running")
}
