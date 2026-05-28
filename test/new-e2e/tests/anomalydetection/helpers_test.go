// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

// Observer log markers emitted by comp/anomalydetection/observer/impl/observer.go
// and comp/anomalydetection/logssource/impl/logssource.go.
const (
	// observerReadyMarker is logged when the demultiplexer wires the observer into
	// the DSD/metrics pipeline. It only appears when metrics.enabled=true: the
	// SetObserver call returns early without calling GetHandle when metrics are off.
	observerReadyMarker = "[observer] getting handle for all-metrics"

	// observerLogsHandleMarker is logged when the logssource component obtains its
	// observer handle. It appears at agent startup when logs.enabled=true, regardless
	// of metrics.enabled. Use this as the readiness signal for log-triggered tests.
	observerLogsHandleMarker = "[observer] getting handle for logs"

	// observerAgentLogsMarker is logged when the agent-log tap is installed.
	observerAgentLogsMarker = "[observer] getting handle for agent-internal-logs"

	// observerReportMarker is printed to stdout (→ journald) by the stdoutReporter
	// on every advance that yields at least one active correlation.
	observerReportMarker = "[observer] report: pattern="

	// metricsDisabledWarning is logged when anomaly_detection.metrics.enabled=false.
	metricsDisabledWarning = "anomaly_detection.metrics.enabled=false"
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
		assert.Contains(c, tel, "observer.series.count", "observer telemetry should expose series gauge when enabled")
		assert.Contains(c, tel, "observer.logs.in_flight", "observer telemetry should expose in-flight logs gauge")
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
		assert.Contains(c, tel, "observer.logs.in_flight", "observer telemetry should expose in-flight logs gauge")
		assert.Contains(c, tel, "log_source=\"containers\"", "observer telemetry should expose containers log_source")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("observer ready (log path)")
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

// dumpObserverLines logs all [observer] journal lines for post-mortem diagnosis.
// SSH errors are silently ignored so this never fails a test.
func dumpObserverLines(t *testing.T, env *environments.Host) {
	t.Helper()
	out, err := env.RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager | grep -F '[observer]' || true")
	if err != nil {
		t.Logf("warning: could not retrieve observer journal lines: %v", err)
		return
	}
	t.Logf("observer lines:\n%s", out)
}
