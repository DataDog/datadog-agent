// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package healthcascade tests that slow checks can cascade into agent liveness
// failures via the collector-queue scheduler blocking mechanism.
//
// Background: the collector scheduler sends checks to workers through an
// unbuffered channel (pendingChecksChan). When all workers are busy running
// slow checks, the channel blocks, the scheduler goroutine hangs, and it
// cannot drain its liveness health channel. After ~30 seconds the health
// system marks the collector-queue as unhealthy, which fails the /live probe
// and causes Kubernetes to kill the pod.
//
// This test proves the full cascade in a real Kubernetes environment:
//
//	Slow HTTP endpoint -> HTTP checks timeout -> workers saturated ->
//	  scheduler blocks -> collector-queue liveness fails -> agent unhealthy
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
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

const (
	slowServerNs        = "default"
	slowServerName      = "slow-http-server"
	slowServerPort      = 8080
	slowServerReplicas  = 10
	slowServerDelaySecs = 20

	// How long the HTTP check waits for a response before timing out.
	// Must be longer than slowServerDelaySecs so the check actually blocks
	// for the full duration rather than failing fast.
	httpCheckTimeoutSecs = 25
)

// helmValues configures the agent for this test:
//   - DD_CHECK_RUNNERS=4: limits workers so we can saturate them with fewer slow checks
//   - Cluster checks enabled to activate the endpoint checks config provider
var helmValues = `
datadog:
  clusterChecks:
    enabled: true
agents:
  containers:
    agent:
      env:
        - name: DD_CHECK_RUNNERS
          value: "4"
`

type slowChecksCascadeSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestSlowChecksCauseLivenessFailure(t *testing.T) {
	e2e.Run(t, &slowChecksCascadeSuite{},
		e2e.WithProvisioner(
			provkindvm.Provisioner(
				provkindvm.WithRunOptions(
					scenariokindvm.WithoutFakeIntake(),
					scenariokindvm.WithAgentOptions(
						kubernetesagentparams.WithHelmValues(helmValues),
					),
				),
			),
		),
	)
}

func (s *slowChecksCascadeSuite) TestSlowChecksSaturateWorkersAndFailLiveness() {
	ctx := context.Background()
	k8s := s.Env().KubernetesCluster.Client()
	kc := s.Env().KubernetesCluster.KubernetesClient

	// ---------------------------------------------------------------
	// Step 1: Wait for the agent to be healthy before we inject faults.
	// ---------------------------------------------------------------
	s.T().Log("Step 1: waiting for agent to be healthy")
	agentPod := s.getAgentPod(ctx, k8s)
	s.T().Logf("Found agent pod: %s", agentPod)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		stdout, _, err := kc.PodExec("datadog", agentPod, "agent", []string{
			"agent", "health",
		})
		assert.NoError(c, err)
		assert.Contains(c, stdout, "Agent health: PASS", "agent should be healthy before fault injection")
	}, 3*time.Minute, 10*time.Second, "agent should be healthy before fault injection")

	// Record the initial restart count so we can detect liveness-induced restarts.
	initialRestarts := s.getAgentRestartCount(ctx, k8s, agentPod)
	s.T().Logf("Initial agent restart count: %d", initialRestarts)

	// ---------------------------------------------------------------
	// Step 2: Deploy slow HTTP server pods annotated for autodiscovery.
	//
	// Each pod runs a Python HTTP server that sleeps 20 seconds before
	// responding. The Datadog autodiscovery annotations configure an
	// http_check instance against each pod. With 10 pods each producing
	// a 20-second check and only 4 workers, the pendingChecksChan will
	// block and the scheduler can't drain its health channel.
	// ---------------------------------------------------------------
	s.T().Log("Step 2: deploying slow HTTP server workload")
	s.deploySlowServer(ctx, k8s)
	defer s.cleanupSlowServer(ctx, k8s)

	// Wait for slow server pods to be running.
	s.EventuallyWithTf(func(c *assert.CollectT) {
		pods, err := k8s.CoreV1().Pods(slowServerNs).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", slowServerName),
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
		assert.GreaterOrEqual(c, ready, slowServerReplicas,
			"need at least %d slow server pods running", slowServerReplicas)
	}, 3*time.Minute, 10*time.Second, "slow server pods should be running")

	// ---------------------------------------------------------------
	// Step 3: Wait for the agent to be killed by the liveness probe.
	//
	// The cascade:
	//   1. Autodiscovery picks up http_check instances from pod annotations
	//   2. Scheduler enqueues checks every ~15 seconds
	//   3. Each check.Run() blocks for ~20 seconds (slow server)
	//   4. All 4 workers become busy -> pendingChecksChan blocks
	//   5. Scheduler goroutine hangs at job.go:207 (blocking send)
	//   6. Health channel not drained -> after ~30s, marked unhealthy
	//   7. K8s liveness probe fails -> pod killed -> restart count increases
	//
	// We detect this by watching the agent pod's restart count increase,
	// which proves K8s killed the pod due to liveness probe failure.
	// We also try to capture the health output before the kill happens.
	// ---------------------------------------------------------------
	s.T().Log("Step 3: waiting for liveness failure (pod restart)")

	// Try to catch the health state before the pod gets killed.
	go func() {
		for i := 0; i < 30; i++ {
			time.Sleep(10 * time.Second)
			stdout, _, err := kc.PodExec("datadog", agentPod, "agent", []string{"agent", "health"})
			if err == nil && strings.Contains(stdout, "collector-queue-") {
				s.T().Logf("Caught unhealthy state before restart:\n%s", stdout)
				return
			}
		}
	}()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		restarts := s.getAgentRestartCount(ctx, k8s, agentPod)
		s.T().Logf("Agent restart count: %d (initial: %d)", restarts, initialRestarts)
		assert.Greater(c, restarts, initialRestarts,
			"agent pod should have been restarted by liveness probe failure")
	}, 8*time.Minute, 15*time.Second,
		"agent pod should be restarted when workers are saturated with slow checks")
}

// getAgentRestartCount returns the restart count for the agent container in the given pod.
func (s *slowChecksCascadeSuite) getAgentRestartCount(ctx context.Context, k8s kubeClient.Interface, podName string) int32 {
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

// getAgentPod returns the name of a running node agent pod.
func (s *slowChecksCascadeSuite) getAgentPod(ctx context.Context, k8s kubeClient.Interface) string {
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
			if !strings.Contains(p.Name, "datadog") {
				continue
			}
			if p.Status.Phase == corev1.PodRunning {
				agentPod = p.Name
				return true
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second, "should find a running agent pod")
	return agentPod
}

// deploySlowServer creates a Deployment of slow HTTP server pods annotated
// for Datadog autodiscovery.
func (s *slowChecksCascadeSuite) deploySlowServer(ctx context.Context, k8s kubeClient.Interface) {
	s.T().Helper()
	replicas := int32(slowServerReplicas)

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
`, slowServerDelaySecs, slowServerPort)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slowServerName,
			Namespace: slowServerNs,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": slowServerName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": slowServerName},
					Annotations: map[string]string{
						"ad.datadoghq.com/slow-server.check_names":  `["http_check"]`,
						"ad.datadoghq.com/slow-server.init_configs": `[{}]`,
						"ad.datadoghq.com/slow-server.instances": fmt.Sprintf(
							`[{"name":"slow_endpoint","url":"http://%%%%host%%%%:%d/","timeout":%d,"min_collection_interval":15}]`,
							slowServerPort, httpCheckTimeoutSecs,
						),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "slow-server",
							Image:   "python:3-slim",
							Command: []string{"python3", "-c", pythonServer},
							Ports: []corev1.ContainerPort{
								{ContainerPort: int32(slowServerPort)},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(slowServerPort),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}

	_, err := k8s.AppsV1().Deployments(slowServerNs).Create(ctx, deploy, metav1.CreateOptions{})
	require.NoError(s.T(), err, "failed to create slow server deployment")
}

// cleanupSlowServer removes the slow server deployment and waits for pods to terminate.
func (s *slowChecksCascadeSuite) cleanupSlowServer(ctx context.Context, k8s kubeClient.Interface) {
	s.T().Helper()
	_ = k8s.AppsV1().Deployments(slowServerNs).Delete(ctx, slowServerName, metav1.DeleteOptions{})
	// Wait for pods to be gone so checks are unscheduled.
	_ = assert.Eventually(s.T(), func() bool {
		pods, err := k8s.CoreV1().Pods(slowServerNs).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", slowServerName),
		})
		return err == nil && len(pods.Items) == 0
	}, 2*time.Minute, 5*time.Second, "slow server pods should be cleaned up")
}
