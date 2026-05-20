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

// waitForObserverReady polls /var/log/datadog/agent.log until the metrics-path
// observer handle marker appears. Only valid when metrics.enabled=true: the
// demultiplexer's SetObserver returns early without calling GetHandle when metrics
// are disabled, so this marker never appears in logs-only mode.
func waitForObserverReady(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for observer to be ready (metrics path)...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		assert.NoError(c, err, "reading agent.log to check observer readiness")
		assert.Contains(c, string(out), observerReadyMarker,
			"agent.log should contain observer startup marker")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("observer ready (metrics path)")
}

// waitForLogsObserverReady polls the systemd journal for the logssource handle marker.
// Use this when metrics.enabled=false: the logssource component calls GetHandle("logs")
// during fx construction (before the file log sink is active), so the marker may appear
// in journald (stderr capture) rather than agent.log. Journalctl is checked here for
// reliability.
func waitForLogsObserverReady(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for observer to be ready (log path)...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute(
			"sudo journalctl -u datadog-agent --no-pager | grep -F '" + observerLogsHandleMarker + "' || true",
		)
		assert.NoError(c, err, "journalctl failed")
		assert.Contains(c, out, observerLogsHandleMarker,
			"journald should contain logssource observer handle marker")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("observer ready (log path)")
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
