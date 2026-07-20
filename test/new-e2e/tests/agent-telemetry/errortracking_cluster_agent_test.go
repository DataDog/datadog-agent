// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// leaderElectionErrorMessage is the exact text logged by KubeASCheck.Run
// (pkg/collector/corechecks/cluster/kubernetesapiserver/kubernetes_apiserver.go)
// when leader election is disabled. Setting DD_LEADER_ELECTION=false makes the
// cluster-agent log this at ERROR level unconditionally on every check run
// (~15s interval, no rate limiter) — a deterministic, dependency-free error
// source for the cluster-agent binary, which runs no Python checks and so
// can't reuse the core-agent suite's error_check.py trigger.
const leaderElectionErrorMessage = "Leader Election not enabled"

// clusterAgentErrorTrackingEnabledHelmValues disables leader election (to
// generate a repeating ERROR log) and enables the errortracking pipeline
// with a fast flush so the wire-shape assertions below run quickly.
const clusterAgentErrorTrackingEnabledHelmValues = `
clusterAgent:
  envDict:
    DD_LEADER_ELECTION: "false"
    DD_AGENT_TELEMETRY_ENABLED: "true"
    DD_AGENT_TELEMETRY_ERRORTRACKING_ENABLED: "true"
    DD_AGENT_TELEMETRY_ERRORTRACKING_FLUSH_INTERVAL_SECONDS: "1"
    DD_AGENT_TELEMETRY_ERRORTRACKING_BOUNCER_WINDOW_SECONDS: "0"
    DD_AGENT_TELEMETRY_ERRORTRACKING_STARTUP_JITTER_SECONDS: "0"
`

// clusterAgentErrorTrackingDisabledHelmValues mirrors the enabled config but
// omits errortracking.enabled, which defaults to false, while still forcing
// the leader-election error so the negative assertion is meaningful.
const clusterAgentErrorTrackingDisabledHelmValues = `
clusterAgent:
  envDict:
    DD_LEADER_ELECTION: "false"
    DD_AGENT_TELEMETRY_ENABLED: "true"
    DD_AGENT_TELEMETRY_ERRORTRACKING_FLUSH_INTERVAL_SECONDS: "1"
    DD_AGENT_TELEMETRY_ERRORTRACKING_BOUNCER_WINDOW_SECONDS: "0"
    DD_AGENT_TELEMETRY_ERRORTRACKING_STARTUP_JITTER_SECONDS: "0"
`

type errorTrackingClusterAgentSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestErrorTrackingClusterAgentSuite is the cluster-agent variant of
// TestAgentTelemetryErrorTrackingSuite: same pkg/util/log/errortracking →
// comp/core/agenttelemetry pipeline, exercised inside the cluster-agent
// binary instead of the core agent.
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

// getClusterAgentPodName returns the name of the (sole) running cluster-agent pod.
func (s *errorTrackingClusterAgentSuite) getClusterAgentPodName() string {
	t := s.T()
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(clusterAgentDatadogNamespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
		Limit:         1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items, "cluster-agent pod not found in datadog namespace")
	return pods.Items[0].Name
}

// TestPayloadShape verifies the happy path: the cluster-agent's own
// leader-election-gated ERROR log reaches FakeIntake with the expected wire
// shape, and — critically for a pipeline about to be shared across many
// binaries — an agent.flavor tag identifying the emitter as cluster_agent
// rather than agent.
func (s *errorTrackingClusterAgentSuite) TestPayloadShape() {
	// testify's suite.Run executes test methods in alphabetical order
	// (TestDisabledByDefault before TestPayloadShape), and UpdateEnv
	// re-provisions the same cluster in place rather than a fresh one.
	// Re-assert the enabled config here so this test doesn't depend on run order.
	s.UpdateEnv(provkind.Provisioner(
		provkind.WithRunOptions(
			scenkind.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(clusterAgentErrorTrackingEnabledHelmValues),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		var err error
		logs, err = s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)
		assert.NotEmpty(c, logs, "no agent-logs records received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for cluster-agent error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l)
		assert.Contains(s.T(), l.Tags, "agent.flavor:"+flavor.ClusterAgent,
			"tags must identify the cluster-agent as the emitting binary; got: %q", l.Tags)
	}
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake even
// though the leader-election error keeps firing locally.
func (s *errorTrackingClusterAgentSuite) TestDisabledByDefault() {
	s.UpdateEnv(provkind.Provisioner(
		provkind.WithRunOptions(
			scenkind.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(clusterAgentErrorTrackingDisabledHelmValues),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	podName := s.getClusterAgentPodName()

	// Clear the log file after resetting FakeIntake so the wait below only matches
	// an occurrence generated after the reset, not a stale one from before it.
	_, _, execErr := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		clusterAgentDatadogNamespace, podName, "cluster-agent",
		[]string{"sh", "-c", "truncate -s 0 /var/log/datadog/cluster-agent.log"})
	require.NoError(s.T(), execErr)

	// Wait until the leader-election error appears in the cluster-agent's own
	// log file, confirming the error is generated locally before asserting it
	// is not forwarded to telemetry.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
			clusterAgentDatadogNamespace, podName, "cluster-agent",
			[]string{"sh", "-c", "awk '/" + leaderElectionErrorMessage + "/{count++} END{print count+0}' /var/log/datadog/cluster-agent.log"})
		assert.NoError(c, err)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 1*time.Minute, 5*time.Second, "timed out waiting for leader-election error to appear in cluster-agent log")

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
