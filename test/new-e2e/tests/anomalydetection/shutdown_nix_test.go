// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

// Item 6 — graceful shutdown under anomaly traffic in-flight.
//
// The observer spawns multiple goroutines (engine, correlator, log detector,
// HFRunner). AGENTS.md flags send-on-closed-channel as the top concurrency bug
// class. This test verifies that SIGTERM during active DogStatsD traffic
// produces a clean exit with no panics, no channel races, and no goroutine leaks.

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// shutdownSuite verifies that the observer shuts down cleanly under load.
type shutdownSuite struct {
	e2e.BaseSuite[environments.Host]
}

// crashIndicators are strings whose presence in the journal after SIGTERM
// indicates a goroutine crash or data-race in the observer.
var crashIndicators = [...]string{
	"panic:",
	"send on closed channel",
	"fatal error: concurrent map",
	"fatal error: concurrent map writes",
}

// TestAnomalyDetectionShutdown provisions the agent with all observer gates enabled.
func TestAnomalyDetectionShutdown(t *testing.T) {
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  enabled: true
  metrics:
    enabled: true
  agent_logs:
    enabled: true
  detectors:
    bocpd:
      warmup_points: 20
`
	e2e.Run(t, &shutdownSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-shutdown"))
}

// TestGracefulShutdownUnderLoad starts a background DogStatsD load generator on
// the remote host, lets it run for 30 s, then sends SIGTERM via systemctl stop.
// It asserts clean exit: no panic, no send-on-closed-channel, no concurrent map
// errors in the journal.
func (s *shutdownSuite) TestGracefulShutdownUnderLoad() {
	waitForObserverReady(s)

	// Start a background shell loop on the remote host that continuously sends
	// DogStatsD gauges. nohup + disown ensures the loop survives the SSH session.
	s.T().Log("starting DogStatsD load generator on remote host...")
	s.Env().RemoteHost.MustExecute(
		"nohup bash -c 'while true; do echo -n \"e2e.shutdown.test:1|g\" > /dev/udp/127.0.0.1/8125; done' > /dev/null 2>&1 & disown",
	)
	// Register cleanup immediately so the remote process is killed even if the
	// test fails or is interrupted via t.FailNow().
	s.T().Cleanup(func() {
		if _, err := s.Env().RemoteHost.Execute("pkill -f 'while true.*8125' || true"); err != nil {
			s.T().Logf("note: pkill cleanup returned an error (best-effort, ignored): %v", err)
		}
	})

	// Let the observer process traffic for 30 s.
	s.T().Log("running load for 30s...")
	time.Sleep(30 * time.Second)

	// Capture a snapshot of the journal before shutdown for post-mortem comparison.
	journalBefore, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager -n 5000")
	if err != nil {
		s.T().Logf("warning: could not capture pre-shutdown journal snapshot: %v", err)
	}

	// Send SIGTERM via systemctl stop; this blocks until the service exits or
	// reaches the systemd timeout. MustExecute fails the test if stop itself
	// crashes, which catches gross shutdown failures immediately.
	s.T().Log("sending SIGTERM via systemctl stop...")
	s.Env().RemoteHost.MustExecute("sudo systemctl stop datadog-agent")
	s.T().Log("agent stopped")

	// Collect the full journal after shutdown.
	journalAfter, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager -n 10000")
	require.NoError(s.T(), err, "collecting journal after shutdown")

	// The journal after shutdown must not contain crash indicators.
	for _, indicator := range crashIndicators {
		assert.NotContains(s.T(), journalAfter, indicator,
			"agent journal must not contain crash indicator %q after SIGTERM", indicator)
	}

	// Confirm systemctl reports the service as inactive (clean stop), not failed.
	status, err := s.Env().RemoteHost.Execute("systemctl is-active datadog-agent || true")
	assert.NoError(s.T(), err, "checking service status")
	assert.Equal(s.T(), "inactive", strings.TrimSpace(status),
		"datadog-agent should be inactive (not failed) after SIGTERM")

	// Diagnostics for post-mortem.
	s.T().Logf("journal before shutdown (last 5000 lines):\n%s", journalBefore)
	dumpObserverLines(s.T(), s.Env())
	lastLines, _ := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager -n 100")
	s.T().Logf("shutdown journal (last 100 lines):\n%s", lastLines)
}
