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

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// Configures 15 HTTP check instances pointing at non-routable IPs
// (192.0.2.0/24, RFC 5737 TEST-NET-1). Each check hangs for its
// timeout (25s), saturating the 4 workers. This avoids deploying extra
// pods (which fail to pull images in Kind CI).
//
// This test PASSES if the agent stays healthy (meaning the slow checks
// don't actually trigger the bug in practice). It FAILS if the agent
// dies (confirming the bug is reachable with enough slow checks).
// ---------------------------------------------------------------------------

// slowCheckInstances returns the YAML for 15 HTTP check instances pointing
// at non-routable IPs (192.0.2.0/24). Each check hangs for 25s, saturating
// the runner's 4 workers.
func slowCheckInstances() string {
	var instances strings.Builder
	for i := 1; i <= 15; i++ {
		if i > 1 {
			instances.WriteString("\n")
		}
		fmt.Fprintf(&instances,
			`        - name: "slow_check_%d"
          url: "http://192.0.2.%d:8080/"
          timeout: 25
          min_collection_interval: 15`, i, i)
	}
	return instances.String()
}

type slowChecksSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestSlowChecksSchedulerBlocking(t *testing.T) {
	e2e.Run(t, &slowChecksSuite{}, kindProvisioner(fmt.Sprintf(`
datadog:
  clusterChecks:
    enabled: true
  confd:
    http_check.yaml: |-
      init_config:
      instances:
%s
agents:
  containers:
    agent:
      env:
        - name: DD_CHECK_RUNNERS
          value: "4"
`, slowCheckInstances())))
}

func (s *slowChecksSuite) TestAgentWithManySlowChecks() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	// Slow checks are configured via Helm (confd http_check.yaml) pointing at
	// non-routable IPs in 192.0.2.0/24. Each check hangs for its 25s timeout,
	// saturating the 4 workers. No extra pods needed.
	s.T().Log("Monitoring agent for 10 minutes with 15 slow HTTP checks (via non-routable IPs, 4 workers)")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 10*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	// This tests the scheduler blocking mechanism. If the agent dies,
	// we've confirmed the bug is reachable in practice.
	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy even with many slow checks. "+
			"If this fails, slow checks triggered the scheduler blocking bug.")
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
	e2e.Run(t, &rcPlusSlowChecksSuite{}, kindProvisioner(fmt.Sprintf(`
datadog:
  clusterChecks:
    enabled: true
  remoteConfiguration:
    enabled: true
  confd:
    http_check.yaml: |-
      init_config:
      instances:
%s
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
        - name: DD_CHECK_RUNNERS
          value: "4"
        - name: DD_REMOTE_CONFIGURATION_RC_DD_URL
          value: "http://192.0.2.1:8080"
        - name: DD_REMOTE_CONFIGURATION_NO_TLS
          value: "true"
        - name: DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL
          value: "5s"
`, slowCheckInstances())))
}

func (s *rcPlusSlowChecksSuite) TestRCHangingWithSlowChecks() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	// Both RC hanging and slow checks are configured at deploy time via Helm.
	// RC points at 192.0.2.1 (hangs), slow checks point at 192.0.2.2-16 (hang for 25s timeout).
	s.T().Log("Monitoring for 10 min: RC hanging for both agents + 15 slow HTTP checks (4 workers)")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 10*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy with RC hanging + slow checks. "+
			"If this fails, the combination of RC unavailability and slow checks is the trigger.")
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

// ---------------------------------------------------------------------------
// Test 7: Full staging simulation — RC hanging with all agent features enabled
//
// Previous tests used a minimal agent (no fakeintake, no logs, no APM, no
// process agent). Staging runs ALL of these, each registering health checks:
//   - forwarder (needs fakeintake/intake to send to)
//   - logs-agent (log collection from all containers)
//   - dogstatsd (metrics receiver)
//   - process-agent (live containers)
//   - APM/trace-agent
//   - tagger, workloadmeta, autodiscovery
//   - slow checks (non-routable IPs)
//
// If the mechanism requires the full agent component set to be running,
// this test should expose it.
// ---------------------------------------------------------------------------

// stagingProvisioner creates a Kind environment with fakeintake enabled
// and all major agent features turned on, matching staging as closely
// as possible.
func stagingProvisioner(helmValues string) e2e.SuiteOption {
	return e2e.WithProvisioner(
		provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				// Fakeintake enabled (default) — forwarder will be active
				scenariokindvm.WithVMOptions(scenec2.WithInstanceType("t3.xlarge")),
				scenariokindvm.WithDeployTestWorkload(),
				scenariokindvm.WithDeployDogstatsd(),
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(helmValues),
				),
			),
		),
	)
}

type stagingSimSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestStagingSimRCHanging(t *testing.T) {
	e2e.Run(t, &stagingSimSuite{}, stagingProvisioner(fmt.Sprintf(`
datadog:
  clusterChecks:
    enabled: true
  remoteConfiguration:
    enabled: true
  logs:
    enabled: true
    containerCollectAll: true
  apm:
    portEnabled: true
  processAgent:
    enabled: true
    processCollection:
      enabled: true
  dogstatsd:
    useHostPort: true
    nonLocalTraffic: true
  networkMonitoring:
    enabled: true
  confd:
    http_check.yaml: |-
      init_config:
      instances:
%s
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
`, slowCheckInstances())))
}

func (s *stagingSimSuite) TestAgentSurvivesStagingConditions() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	pod := getNodeAgentPod(ctx, s.T(), k8s)
	s.T().Logf("Node agent pod: %s", pod)

	s.T().Log("Monitoring for 15 min: RC hanging + all features + slow checks + workloads + fakeintake")
	result := monitorAgent(ctx, s.T(), kc, k8s, pod, 15*time.Minute, 15*time.Second)
	reportResults(s.T(), result)

	assert.False(s.T(), result.AgentDied,
		"Agent should stay healthy under staging-like conditions with RC hanging. "+
			"If this fails, the full component set + RC unavailability is the trigger.")
}
