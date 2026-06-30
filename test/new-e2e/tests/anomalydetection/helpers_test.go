// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// baselineAnalysisDisabledYAML must be included in the agent config of any E2E
// test that waits for anomaly output (reports, severity escalations, etc.).
//
// The default 10-minute baseline window (anomaly_detection.baseline_analysis.enabled=true)
// silently drops all detector anomalies while it is active — tests will always time out
// unless this snippet is embedded under the anomaly_detection: block.
//
// Usage:
//
//	agentConfig := `
//	anomaly_detection:
//	  enabled: true
//	  ...
//	` + baselineAnalysisDisabledYAML
const baselineAnalysisDisabledYAML = `  baseline_analysis:
    enabled: false
`

// Canonical observer telemetry names.
const (
	telemetrySeriesCount    = "observer.series.count"
	telemetryLogsInFlight   = "observer.logs.in_flight"
	telemetryLogsIngested   = "observer.logs.ingested"
	telemetryReportsEmitted = "observer.reports.emitted"
	telemetryReportsOngoing = "observer.reports.ongoing"

	// scorerHelperEscalationMarker is emitted by anomalyScorer.OnSeverityTransition
	// when output.logs=true and the EWMA rises above low_threshold (an escalation event).
	// Logged at info level, captured by journald, and serves as the assertion target.
	// Full example: "[observer] anomaly scorer anomaly_scorer severity escalation to Medium (was Low, t=...)"
	scorerHelperEscalationMarker = "[observer] anomaly scorer anomaly_scorer severity escalation"

	// scorerHelperRegisteredMarker is logged once at agent startup when the
	// anomaly scorer is successfully wired with telemetry. Waiting for it
	// before sending metrics ensures the scorer is active.
	scorerHelperRegisteredMarker = "[observer] anomaly_scorer registered"
)

// observerTestSuite is a minimal interface satisfied by all suite types in this
// package. It gives helpers access to the test object, EventuallyWithT, and the
// provisioned environment without depending on a concrete suite type.
type observerTestSuite interface {
	T() *testing.T
	EventuallyWithT(condition func(*assert.CollectT), waitFor, tick time.Duration, msgAndArgs ...any) bool
	Env() *environments.Host
}

// waitForObserverReady waits for observer telemetry to appear in agent-full-telemetry.
// This avoids startup-log parsing and works across both service log sinks.
func waitForObserverReady(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for observer to be ready (metrics path)...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady(), "agent should be ready")
		tel := observerTelemetryOutput(s)
		assert.True(c, containsMetric(tel, telemetrySeriesCount),
			"observer telemetry should expose series gauge when enabled")
		assert.True(c, containsMetric(tel, telemetryLogsInFlight),
			"observer telemetry should expose in-flight logs gauge")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("observer ready (metrics path)")
}

// waitForLogsObserverReady waits for log-source telemetry dimensions to appear.
func waitForLogsObserverReady(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for observer to be ready (log path)...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady(), "agent should be ready")
		tel := observerTelemetryOutput(s)
		assert.True(c, containsMetric(tel, telemetryLogsInFlight),
			"observer telemetry should expose in-flight logs gauge")
		assert.True(c, containsAny(tel, "log_source=\"containers\"", "log_source:containers"),
			"observer telemetry should expose containers log_source")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("observer ready (log path)")
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func metricNameVariants(metric string) []string {
	snake := strings.ReplaceAll(metric, ".", "_")
	variants := []string{
		metric,
		snake,
		"observer__" + snake,
	}
	return slices.Compact(variants)
}

func containsMetric(haystack, metric string) bool {
	return containsAny(haystack, metricNameVariants(metric)...)
}

func containsMetricWithTag(haystack, metric, key, value string) bool {
	if !containsMetric(haystack, metric) {
		return false
	}
	return containsAny(haystack,
		fmt.Sprintf("%s=\"%s\"", key, value),
		fmt.Sprintf("%s:%s", key, value),
		fmt.Sprintf("%s=%s", key, value),
	)
}

func observerTelemetryOutput(s observerTestSuite) string {
	s.T().Helper()
	return s.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "agent-full-telemetry"}))
}

// waitForAgentStartup polls agent.log for the standard startup banner.
func waitForAgentStartup(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for agent to start...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		assert.NoError(c, err, "reading agent.log")
		assert.Contains(c, string(out), "Starting Datadog Agent",
			"agent.log should contain startup marker")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("agent started")
}

// waitForScorerHelperReady polls the agent journal until the scorer helper
// registration log line appears, confirming the helper is subscribed and will
// receive OnSeverityTransition callbacks before metrics are sent.
func waitForScorerHelperReady(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for anomaly scorer helper to be registered...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, scorerHelperRegisteredMarker,
			"journal should contain the scorer helper registration log line")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("anomaly scorer helper registered")
}

func waitForReportsTelemetry(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for observer reports telemetry...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		assert.True(c, s.Env().Agent.Client.IsReady(), "agent should be ready")
		tel := observerTelemetryOutput(s)
		assert.True(c, containsMetric(tel, telemetryReportsEmitted),
			"observer telemetry should expose reports emitted counter after anomalies")
	}, 3*time.Minute, 5*time.Second)
	s.T().Log("observer reports telemetry detected")
}
