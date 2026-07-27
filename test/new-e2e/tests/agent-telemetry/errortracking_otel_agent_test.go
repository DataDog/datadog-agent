// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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

// otelScrapeErrorMessage is logged by scraperhelper whenever a scraper's
// Scrape() call errors; the kubeletstats receiver here points at a
// connection-refused address, firing deterministically every interval.
const otelScrapeErrorMessage = "Error scraping metrics"

// errorTrackingOTelAgentEnabledHelmValues enables the errortracking pipeline
// on the otel-agent container with a fast flush. The actual ERROR log comes
// from the kubeletstats receiver in errorTrackingOTelCollectorConfig.
const errorTrackingOTelAgentEnabledHelmValues = `
datadog:
  otelCollector:
    useStandaloneImage: false
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
datadog:
  otelCollector:
    useStandaloneImage: false
agents:
  containers:
    otelAgent:
      envDict:
        DD_AGENT_TELEMETRY_ENABLED: "true"
        DD_AGENT_TELEMETRY_ERRORTRACKING_FLUSH_INTERVAL_SECONDS: "1"
        DD_AGENT_TELEMETRY_ERRORTRACKING_BOUNCER_WINDOW_SECONDS: "0"
        DD_AGENT_TELEMETRY_ERRORTRACKING_STARTUP_JITTER_SECONDS: "0"
`

// withOTelAgentTelemetryFakeintake points the otel-agent container's
// agent-telemetry logs endpoint at FakeIntake. Unlike DD_LOGS_CONFIG_LOGS_DD_URL,
// agent_telemetry.logs_dd_url has no framework helper for Helm installs.
func withOTelAgentTelemetryFakeintake() kubernetesagentparams.Option {
	return func(p *kubernetesagentparams.Params) error {
		if p.FakeIntake == nil {
			return errors.New("withOTelAgentTelemetryFakeintake requires WithFakeintake")
		}
		asset := p.FakeIntake.URL.ApplyT(func(url string) (pulumi.Asset, error) {
			return pulumi.NewStringAsset(fmt.Sprintf(`
agents:
  containers:
    otelAgent:
      envDict:
        DD_AGENT_TELEMETRY_LOGS_DD_URL: %q
        DD_AGENT_TELEMETRY_LOGS_NO_SSL: "true"
`, url)), nil
		}).(pulumi.AssetOutput)
		p.HelmValues = append(p.HelmValues, asset)
		return nil
	}
}

type errorTrackingOTelAgentSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestErrorTrackingOTelAgentSuite is the otel-agent variant of
// TestAgentTelemetryErrorTrackingSuite, exercised via the OTel collector's
// own zap-bridged logging rather than the core agent's.
func TestErrorTrackingOTelAgentSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingOTelAgentSuite{},
		e2e.WithProvisioner(provkind.Provisioner(
			provkind.WithRunOptions(
				scenkind.WithAgentOptions(
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(errorTrackingOTelCollectorConfig),
					kubernetesagentparams.WithHelmValues(errorTrackingOTelAgentEnabledHelmValues),
					withOTelAgentTelemetryFakeintake(),
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

// TestPayloadShape verifies the otel-agent's own kubeletstats scrape-error
// ERROR log reaches FakeIntake tagged agent.flavor:otel_agent. Filtered by
// stack trace since the core agent sharing this pod could forward its own errors too.
func (s *errorTrackingOTelAgentSuite) TestPayloadShape() {
	// BeforeTest already reset the environment to the suite's original
	// (enabled) provisioner regardless of run order, and the scrape error
	// recurs on every collection_interval, so no re-provisioning is needed.
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
		assertCommonLogShape(s.T(), l, flavor.OTelAgent)
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
				withOTelAgentTelemetryFakeintake(),
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
