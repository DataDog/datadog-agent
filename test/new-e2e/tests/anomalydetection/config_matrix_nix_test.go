// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

// Item 7 — sub-gate independence matrix (observer gate on in all cases).
//
// Three independent sub-gates (metrics.enabled, logs.enabled, logs.internal.enabled)
// sit under anomaly_detection.anomaly_scorer.dry_run.enabled=true. This file verifies
// that each sub-gate activates or suppresses the correct path independently.
// The all-gates-off case is covered by defaults_nix_test.go.
//
// Each case has its own suite type so exactly one test method runs per provisioned
// VM. Sharing a suite type across entry points causes every method on the suite to
// run for every provisioner config, producing cross-contamination failures.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// --- Case 1: metrics=off, logs=off ---------------------------------------

type metricsOffLogsOffSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixMetricsOffLogsOff verifies the observer starts but
// emits the deterministic metrics-disabled warning and no agent-logs handle.
func TestObserverConfigMatrixMetricsOffLogsOff(t *testing.T) {
	t.Parallel()
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
  metrics:
    enabled: false
  logs:
    enabled: false
    internal:
      enabled: false
`
	e2e.Run(t, &metricsOffLogsOffSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-both-off"))
}

// TestWarningPresent guards against silent processing when both sub-gates are
// disabled. No log ingestion telemetry should be emitted.
func (s *metricsOffLogsOffSuite) TestWarningPresent() {
	waitForObserverReady(s)
	time.Sleep(10 * time.Second)

	tel := observerTelemetryOutput(s)
	assert.False(s.T(), containsMetric(tel, telemetryLogsIngested),
		"no log ingestion telemetry expected when logs and logs.internal are disabled")
	assert.False(s.T(), containsMetric(tel, telemetryReportsEmitted),
		"no reports expected when both metrics and logs ingestion are disabled")
}

// --- Case 2: metrics=on, logs=off ----------------------------------------

type metricsOnLogsOffSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixMetricsOnLogsOff verifies the metrics path is active
// and the agent-logs tap is not installed.
func TestObserverConfigMatrixMetricsOnLogsOff(t *testing.T) {
	t.Parallel()
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
  metrics:
    enabled: true
  logs:
    enabled: false
    internal:
      enabled: false
`
	e2e.Run(t, &metricsOnLogsOffSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-metrics-on"))
}

// TestSubGateIndependence verifies that enabling the metrics path does not
// silently enable the agent-log tap.
func (s *metricsOnLogsOffSuite) TestSubGateIndependence() {
	waitForObserverReady(s)
	tel := observerTelemetryOutput(s)
	assert.True(s.T(), containsMetric(tel, telemetrySeriesCount),
		"metrics path should expose series telemetry when enabled")
	assert.False(s.T(), containsMetricWithTag(tel, telemetryLogsIngested, "log_source", "internal"),
		"internal log ingestion should not be active when logs.internal is disabled")
}

// --- Case 3: metrics=off, logs=on ----------------------------------------

type metricsOffLogsOnSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixMetricsOffLogsOn verifies the agent-log tap is
// installed and the metrics-disabled warning appears.
func TestObserverConfigMatrixMetricsOffLogsOn(t *testing.T) {
	t.Parallel()
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
  metrics:
    enabled: false
  logs:
    internal:
      enabled: true
`
	e2e.Run(t, &metricsOffLogsOnSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-logs-on"))
}

// TestLogTapActiveMetricsWarningPresent verifies internal log ingestion telemetry
// appears when logs.internal.enabled=true and metrics ingest is disabled.
func (s *metricsOffLogsOnSuite) TestLogTapActiveMetricsWarningPresent() {
	waitForObserverReady(s)
	s.EventuallyWithT(func(c *assert.CollectT) {
		tel := observerTelemetryOutput(s)
		assert.True(c, containsMetricWithTag(tel, telemetryLogsIngested, "log_source", "internal"),
			"internal log ingestion should be active when logs.internal is enabled")
	}, 2*time.Minute, 3*time.Second)
}

// --- Case 4: dry-run gate only (default sub-gates on) -------------------

type masterOnlyDefaultsOnSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixDryRunOnlyDefaultsOn verifies that enabling
// an observer gate implicitly keeps the metrics and internal log paths active
// through their default=true sub-gates.
func TestObserverConfigMatrixDryRunOnlyDefaultsOn(t *testing.T) {
	t.Parallel()
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
`
	e2e.Run(t, &masterOnlyDefaultsOnSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-dry-run-defaults-on"))
}

// TestBothPathsActive verifies that the metrics and internal log paths both
// start when only an observer gate is enabled.
func (s *masterOnlyDefaultsOnSuite) TestBothPathsActive() {
	waitForObserverReady(s)
	s.EventuallyWithT(func(c *assert.CollectT) {
		tel := observerTelemetryOutput(s)
		assert.True(c, containsMetric(tel, telemetrySeriesCount),
			"metrics path should be active")
		assert.True(c, containsMetricWithTag(tel, telemetryLogsIngested, "log_source", "internal"),
			"agent-logs ingestion should be active")
	}, 2*time.Minute, 3*time.Second)
}
