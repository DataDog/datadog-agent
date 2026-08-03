// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

// clusterAgentDatadogNamespace is the namespace the kind provisioner installs
// the datadog-agent Helm release into.
const clusterAgentDatadogNamespace = "datadog"

// leaderElectionErrorMessage is logged at ERROR on every check run once
// leader election is disabled, a deterministic trigger for this suite.
const leaderElectionErrorMessage = "Leader Election not enabled"

// clusterAgentErrorTrackingEnabledHelmValues disables leader election and
// enables the errortracking pipeline with a fast flush.
//
//go:embed testdata/errortracking-cluster-agent-enabled.yaml
var clusterAgentErrorTrackingEnabledHelmValues string

// clusterAgentErrorTrackingDisabledHelmValues mirrors the enabled config but
// omits errortracking.enabled, which defaults to false.
//
//go:embed testdata/errortracking-cluster-agent-disabled.yaml
var clusterAgentErrorTrackingDisabledHelmValues string

type errorTrackingClusterAgentSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestErrorTrackingClusterAgentSuite is the cluster-agent variant of
// TestAgentTelemetryErrorTrackingSuite.
func TestErrorTrackingClusterAgentSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingClusterAgentSuite{},
		e2e.WithProvisioner(provkind.Provisioner(
			provkind.WithRunOptions(
				scenkind.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(clusterAgentErrorTrackingEnabledHelmValues),
				),
			),
		)),
	)
}

// getClusterAgentPodNames returns the names of all running cluster-agent
// pods. This suite runs clusterAgent.replicas: 2, so callers must not assume a single pod.
func (s *errorTrackingClusterAgentSuite) getClusterAgentPodNames() []string {
	t := s.T()
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(clusterAgentDatadogNamespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items, "no cluster-agent pods found in datadog namespace")
	names := make([]string, 0, len(pods.Items))
	for _, pod := range pods.Items {
		names = append(names, pod.Name)
	}
	return names
}

// getClusterAgentContainerLogs returns the "cluster-agent" container's stdout
// for podName, restricted to entries logged at or after since.
func (s *errorTrackingClusterAgentSuite) getClusterAgentContainerLogs(ctx context.Context, podName string, since time.Time) (string, error) {
	sinceTime := metav1.NewTime(since)
	stream, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(clusterAgentDatadogNamespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: "cluster-agent",
		SinceTime: &sinceTime,
	}).Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	out, err := io.ReadAll(stream)
	return string(out), err
}

// TestPayloadShape verifies the cluster-agent's own leader-election-gated
// ERROR log reaches FakeIntake with an agent.flavor tag of cluster_agent.
func (s *errorTrackingClusterAgentSuite) TestPayloadShape() {
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		var err error
		logs, err = s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)
		assert.NotEmpty(c, logs, "no agent-logs records received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for cluster-agent error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l, flavor.ClusterAgent)
	}
}

// TestDisabledByDefault verifies that when errortracking omits `enabled`
// (defaulting to false), no agent-logs records reach FakeIntake.
func (s *errorTrackingClusterAgentSuite) TestDisabledByDefault() {
	s.UpdateEnv(provkind.Provisioner(
		provkind.WithRunOptions(
			scenkind.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(clusterAgentErrorTrackingDisabledHelmValues),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	ctx := s.T().Context()
	podNames := s.getClusterAgentPodNames()
	since := time.Now()

	// Wait until the leader-election error appears in at least one replica's
	// stdout before asserting it is not forwarded to telemetry.
	ok := assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		statuses := make([]string, 0, len(podNames))
		for _, podName := range podNames {
			out, err := s.getClusterAgentContainerLogs(ctx, podName, since)
			if err == nil && strings.Contains(out, leaderElectionErrorMessage) {
				return
			}
			if err != nil {
				statuses = append(statuses, fmt.Sprintf("%s: log fetch error: %v", podName, err))
			} else {
				statuses = append(statuses, podName+": message not found")
			}
		}
		assert.Fail(c, "leader-election error not yet found in any cluster-agent replica's log: "+strings.Join(statuses, "; "))
	}, 2*time.Minute, 5*time.Second, "timed out waiting for leader-election error to appear in cluster-agent log")

	// On failure, dump each replica's log tail for debugging.
	if !ok {
		for _, podName := range podNames {
			out, err := s.getClusterAgentContainerLogs(ctx, podName, since)
			if err != nil {
				s.T().Logf("could not read %s cluster-agent logs: %v", podName, err)
				continue
			}
			s.T().Logf("=== %s cluster-agent log since %s ===\n%s", podName, since, out)
		}
		s.T().FailNow()
	}

	// Confirm nothing is forwarded across five flush cycles (flush_interval_seconds: 1).
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
