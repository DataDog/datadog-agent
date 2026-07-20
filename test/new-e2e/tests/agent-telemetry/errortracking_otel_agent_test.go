// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	_ "embed"
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

// otelAgentDatadogNamespace is the namespace the kind provisioner installs
// the datadog-agent Helm release into.
const otelAgentDatadogNamespace = "datadog"

//go:embed testdata/errortracking-otel-agent-collector-config.yaml
var errorTrackingOTelCollectorConfig string

// otelScrapeErrorMessage is the text logged by go.opentelemetry.io/collector's
// scraperhelper (obs_metrics.go) whenever a scraper's Scrape() call returns an
// error. The kubeletstats receiver in errorTrackingOTelCollectorConfig is
// pointed at a connection-refused address, so it fails deterministically on
// every collection_interval — a dependency-free error source for the
// otel-agent binary, which runs no checks or HTTP receiver of its own. All
// OTel collector zap logging (any receiver/processor/exporter/extension) is
// bridged into pkg/util/log by comp/otelcol/collector/impl's zap.WrapCore, so
// this reaches the same errortracking pipeline as any other binary's logs.
const otelScrapeErrorMessage = "Error scraping metrics"

// errorTrackingOTelAgentEnabledHelmValues enables the errortracking pipeline
// on the otel-agent container with a fast flush so the wire-shape assertions
// below run quickly. The kubeletstats receiver in
// errorTrackingOTelCollectorConfig generates the actual ERROR log
// independent of this config.
const errorTrackingOTelAgentEnabledHelmValues = `
agents:
  containers:
    otelAgent:
      envDict:
        DD_AGENT_TELEMETRY_ENABLED: "true"
        DD_AGENT_TELEMETRY_ERRORTRACKING_ENABLED: "true"
        DD_AGENT_TELEMETRY_ERRORTRACKING_FLUSH_INTERVAL_SECONDS: "1"
        DD_AGENT_TELEMETRY_ERRORTRACKING_BOUNCER_WINDOW_SECONDS: "0"
        DD_AGENT_TELEMETRY_ERRORTRACKING_STARTUP_JITTER_SECONDS: "0"
`

// errorTrackingOTelAgentDisabledHelmValues mirrors the enabled config but
// omits errortracking.enabled, which defaults to false, while still forcing
// the kubeletstats scrape error so the negative assertion is meaningful.
const errorTrackingOTelAgentDisabledHelmValues = `
agents:
  containers:
    otelAgent:
      envDict:
        DD_AGENT_TELEMETRY_ENABLED: "true"
        DD_AGENT_TELEMETRY_ERRORTRACKING_FLUSH_INTERVAL_SECONDS: "1"
        DD_AGENT_TELEMETRY_ERRORTRACKING_BOUNCER_WINDOW_SECONDS: "0"
        DD_AGENT_TELEMETRY_ERRORTRACKING_STARTUP_JITTER_SECONDS: "0"
`

type errorTrackingOTelAgentSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestErrorTrackingOTelAgentSuite is the otel-agent variant of
// TestAgentTelemetryErrorTrackingSuite: same pkg/util/log/errortracking →
// comp/core/agenttelemetry pipeline, exercised via the OTel collector's own
// zap-bridged logging inside the otel-agent binary instead of the core agent.
func TestErrorTrackingOTelAgentSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingOTelAgentSuite{},
		e2e.WithProvisioner(provkind.Provisioner(
			provkind.WithRunOptions(
				scenkind.WithAgentOptions(
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(errorTrackingOTelCollectorConfig),
					kubernetesagentparams.WithHelmValues(errorTrackingOTelAgentEnabledHelmValues),
				),
			),
		)),
	)
}

// getNodeAgentPodName returns the name of the (sole) running node-agent pod.
// The otel-agent under test runs as the "otel-agent" container within this
// same pod, alongside the core "agent" container.
func (s *errorTrackingOTelAgentSuite) getNodeAgentPodName() string {
	t := s.T()
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(otelAgentDatadogNamespace).List(t.Context(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
		Limit:         1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items, "node-agent pod not found in datadog namespace")
	return pods.Items[0].Name
}

// TestPayloadShape verifies the happy path: the otel-agent's own
// kubeletstats scrape-error ERROR log reaches FakeIntake with the expected
// wire shape, and — critically for a pipeline shared across many binaries —
// an agent.flavor tag identifying the emitter as otel_agent rather than
// agent. Records are filtered by stack trace rather than assumed exclusive,
// since the core agent sharing this pod could in principle forward its own,
// unrelated errors during the same window.
func (s *errorTrackingOTelAgentSuite) TestPayloadShape() {
	// testify's suite.Run executes test methods in alphabetical order
	// (TestDisabledByDefault before TestPayloadShape), and UpdateEnv
	// re-provisions the same cluster in place rather than a fresh one.
	// Re-assert the enabled config here so this test doesn't depend on run order.
	s.UpdateEnv(provkind.Provisioner(
		provkind.WithRunOptions(
			scenkind.WithAgentOptions(
				kubernetesagentparams.WithOTelAgent(),
				kubernetesagentparams.WithOTelConfig(errorTrackingOTelCollectorConfig),
				kubernetesagentparams.WithHelmValues(errorTrackingOTelAgentEnabledHelmValues),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		all, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logs = nil
		for _, l := range all {
			if strings.Contains(l.StackTrace, "scraperhelper") {
				logs = append(logs, l)
			}
		}
		assert.NotEmpty(c, logs, "no otel-agent scrape error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for otel-agent error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l)
		assert.Contains(s.T(), l.Tags, "agent.flavor:"+flavor.OTelAgent,
			"tags must identify the otel-agent as the emitting binary; got: %q", l.Tags)
	}
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake even
// though the kubeletstats scrape error keeps firing locally.
func (s *errorTrackingOTelAgentSuite) TestDisabledByDefault() {
	s.UpdateEnv(provkind.Provisioner(
		provkind.WithRunOptions(
			scenkind.WithAgentOptions(
				kubernetesagentparams.WithOTelAgent(),
				kubernetesagentparams.WithOTelConfig(errorTrackingOTelCollectorConfig),
				kubernetesagentparams.WithHelmValues(errorTrackingOTelAgentDisabledHelmValues),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	podName := s.getNodeAgentPodName()

	// Clear the log file after resetting FakeIntake so the wait below only matches
	// an occurrence generated after the reset, not a stale one from before it.
	_, _, execErr := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		otelAgentDatadogNamespace, podName, "otel-agent",
		[]string{"sh", "-c", "truncate -s 0 /var/log/datadog/otel-agent.log"})
	require.NoError(s.T(), execErr)

	// Wait until the scrape error appears in the otel-agent's own log file,
	// confirming the error is generated locally before asserting it is not
	// forwarded to telemetry.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
			otelAgentDatadogNamespace, podName, "otel-agent",
			[]string{"sh", "-c", "awk '/" + otelScrapeErrorMessage + "/{count++} END{print count+0}' /var/log/datadog/otel-agent.log"})
		assert.NoError(c, err)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 1*time.Minute, 5*time.Second, "timed out waiting for scrape error to appear in otel-agent log")

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
