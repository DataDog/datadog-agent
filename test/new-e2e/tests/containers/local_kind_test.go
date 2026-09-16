// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file adds an entry point to the containers test suite that runs
// against a local kind cluster managed by e2ectl (e2ectl start --base kind +
// e2ectl install) instead of provisioning via Pulumi. It asserts what makes
// sense on a Helm-installed agent in a bare kind cluster; the workload,
// KSM, HPA and leader-election tests in the full k8sSuite still need the
// Pulumi-provisioned environment.
package containers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fakeintakeaggregator "github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// localKindSuite is a standalone suite (not inheriting baseSuite, which
// carries Datadog-API event dependencies) that works on any kind cluster
// with the Helm-installed agent.
type localKindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]

	clusterName string
	fi          *fakeintakeclient.Client
}

// TestContainersOnLocalKind runs against a local kind cluster managed by
// e2ectl (e2ectl start --base kind, then e2ectl install). It attaches to
// the existing environment via the snapshot; no Pulumi, no provisioning,
// no teardown.
func TestContainersOnLocalKind(t *testing.T) {
	envName := os.Getenv("E2ECTL_ENV")
	if envName == "" {
		t.Skip("set E2ECTL_ENV to a running e2ectl kind environment (e2ectl start --base kind, then e2ectl install)")
	}
	home := os.Getenv("E2ECTL_HOME")
	if home == "" {
		home = os.ExpandEnv("$HOME/.e2ectl")
	}
	snapshot := filepath.Join(home, "envs", envName, "snapshot.json")
	if _, err := os.Stat(snapshot); err != nil {
		t.Skipf("no snapshot for %s (e2ectl start first): %v", envName, err)
	}
	t.Parallel()
	e2e.Run(t, &localKindSuite{}, e2e.WithProvisioner(
		provisioners.NewStaticStackProvisioner[environments.Kubernetes]("e2ectl-attach", snapshot),
	))
}

func (suite *localKindSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	suite.clusterName = suite.Env().KubernetesCluster.ClusterName
	suite.fi = suite.Env().FakeIntake.Client()
}

// TestAgentRunning verifies the Helm-installed agent pods are Ready.
func (suite *localKindSuite) TestAgentRunning() {
	ctx := suite.T().Context()
	suite.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
			LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
		})
		require.NoErrorf(c, err, "Failed to list agent pods")
		require.NotEmpty(c, pods.Items, "No agent pods found")
		for _, pod := range pods.Items {
			for _, cs := range pod.Status.ContainerStatuses {
				assert.Truef(c, cs.Ready, "Container %s of pod %s isn't Ready", cs.Name, pod.Name)
				assert.Zerof(c, cs.RestartCount, "Container %s of pod %s has restarted", cs.Name, pod.Name)
			}
		}
	}, 5*time.Minute, 10*time.Second, "Agent pods didn't become Ready in time")
}

// TestAgentHeartbeat verifies the heartbeat metric reaches the fakeintake.
func (suite *localKindSuite) TestAgentHeartbeat() {
	suite.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := suite.fi.FilterMetrics("datadog.agent.running")
		require.NoErrorf(c, err, "Failed to filter metrics")
		require.NotEmptyf(c, metrics, "No heartbeat metric yet")
	}, 2*time.Minute, 15*time.Second, "Agent heartbeat not found in fakeintake")
}

// TestSystemCPU verifies the system.cpu.user core check is emitting —
// the kind cluster's own kubelet provides the runtime for the agent to
// collect from.
func (suite *localKindSuite) TestSystemCPU() {
	// No kube_cluster_name tag assertion: that tag is applied by the Pulumi
	// provisioner via DD_TAGS, not by the minimal Helm install.
	suite.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := suite.fi.FilterMetrics("system.cpu.user")
		require.NoErrorf(c, err, "Failed to filter cpu metrics")
		require.NotEmptyf(c, metrics, "No system.cpu.user metric yet")
	}, 2*time.Minute, 15*time.Second, "system.cpu.user not found in fakeintake")
}

// TestAgentCLIVersion runs `agent version` inside the agent pod.
func (suite *localKindSuite) TestAgentCLIVersion() {
	ctx := context.Background()
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
		Limit:         1,
	})
	suite.Require().NoError(err, "Failed to list agent pods")
	suite.Require().NotEmpty(pods.Items, "No agent pods found")

	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog", pods.Items[0].Name, "agent", []string{"agent", "version"})
	suite.Require().NoError(err)
	suite.Empty(stderr, "agent version stderr should be empty")
	suite.Contains(stdout, "Agent ", "agent version should identify itself")
}

// TestAgentCLIStatus runs `agent status` inside the agent pod.
func (suite *localKindSuite) TestAgentCLIStatus() {
	ctx := context.Background()
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
		Limit:         1,
	})
	suite.Require().NoError(err, "Failed to list agent pods")
	suite.Require().NotEmpty(pods.Items, "No agent pods found")

	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		"datadog", pods.Items[0].Name, "agent", []string{"agent", "status"})
	suite.Require().NoError(err)
	suite.Empty(stderr, "agent status stderr should be empty")
	suite.Contains(stdout, "Running Checks")
	suite.Contains(stdout, "Instance ID: cpu [OK]", "agent status should show the cpu core check running")
}

// TestFakeintakeReachable verifies the fakeintake is queryable.
func (suite *localKindSuite) TestFakeintakeReachable() {
	suite.Require().NoError(suite.fi.GetServerHealth(), "fakeintake should be reachable")
}

// TestWorkloadRunning verifies the workload deployed by e2ectl's
// workloads section is Running in its catalog namespace. This is the
// deployment-side half of the workloads feature: the same declaration
// deploys via kubectl on kind.
func (suite *localKindSuite) TestWorkloadRunning() {
	ctx := suite.T().Context()
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("workload-nginx").List(ctx, metav1.ListOptions{})
	suite.Require().NoError(err, "Failed to list workload pods — was a workload declared in the e2ectl config?")
	suite.Require().NotEmpty(pods.Items, "No nginx workload pods found — add 'workloads: [{app: nginx}]' to the e2ectl config")
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			suite.Require().Truef(cs.Ready, "Workload container %s of pod %s isn't Ready", cs.Name, pod.Name)
		}
	}
}

// TestWorkloadContainerMetrics verifies the agent monitors the workload
// container: container metrics for the nginx pod reach the fakeintake,
// tagged with the Kubernetes workload identity. This proves the whole
// loop — workload deployed, agent sees it, metrics flow.

// TestWorkloadADCheck verifies the AD annotation on the nginx workload
// pod triggers the nginx integration check: nginx.* metrics reach the
// fakeintake. This is the catalog's reason for existing — the annotation
// is applied by the workload deployer, not hand-written by the test.
func (suite *localKindSuite) TestWorkloadContainerMetrics() {
	suite.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := suite.fi.FilterMetrics("container.cpu.usage",
			fakeintakeclient.WithTags[*fakeintakeaggregator.MetricSeries]([]string{"kube_container_name:nginx"}))
		require.NoErrorf(c, err, "Failed to filter container metrics")
		require.NotEmptyf(c, metrics, "No container.cpu.usage for the nginx workload yet")
	}, 3*time.Minute, 15*time.Second, "Agent is not monitoring the nginx workload")
}

func (suite *localKindSuite) TestWorkloadADCheck() {
	suite.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := suite.fi.FilterMetrics("nginx.net.connections")
		require.NoErrorf(c, err, "Failed to filter nginx metrics")
		require.NotEmptyf(c, metrics, "No nginx.net.connections yet — AD check from the workload annotation not running")
	}, 3*time.Minute, 15*time.Second, "nginx integration check (AD annotation) is not emitting")
}
