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

// Observer log markers emitted by comp/anomalydetection/observer/impl/observer.go.
const (
	// observerReadyMarker is logged when the observer is wired into the aggregator.
	// Absence means anomaly_detection.enabled=false or the reporter impl is not loaded.
	observerReadyMarker = "[observer] getting handle for all-metrics"

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

// waitForObserverReady polls /var/log/datadog/agent.log until the observer startup
// marker appears. Fails the test if the marker is not seen within 2 minutes.
func waitForObserverReady(s observerTestSuite) {
	s.T().Helper()
	s.T().Log("waiting for observer to be ready...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		assert.NoError(c, err, "reading agent.log to check observer readiness")
		assert.Contains(c, string(out), observerReadyMarker,
			"agent.log should contain observer startup marker")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("observer ready")
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
	out, err := env.RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager -n 10000 | grep -F '[observer]' || true")
	if err != nil {
		t.Logf("warning: could not retrieve observer journal lines: %v", err)
		return
	}
	t.Logf("observer lines:\n%s", out)
}
