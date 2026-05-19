// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package anomalydetection contains E2E tests for the anomaly detection observer.
// Each file in this package covers one concern:
//
//   - anomalydetection_nix_test.go — DSD-spike reporter (BOCPD, stdout marker)
//   - log_triggered_nix_test.go   — log-driven reporter (CUSUM, http_check errors)
//   - defaults_nix_test.go        — observer disabled by default (no [observer] lines)
//   - config_matrix_nix_test.go   — sub-gate independence (metrics/logs/agent_logs gates)
//   - shutdown_nix_test.go        — graceful shutdown under DSD load (no panic/crash)
package anomalydetection

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// stdoutReporterSuite exercises the DSD-metrics path of the observer. It sends a
// stable gauge baseline followed by a large spike to trip the BOCPD detector, then
// asserts the canonical "[observer] report: pattern=" marker appears in the agent's
// systemd journal (stdout → journald by default).
type stdoutReporterSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionStdoutReporter provisions a Linux VM with the observer
// enabled and the BOCPD warmup tuned for a short test window.
func TestAnomalyDetectionStdoutReporter(t *testing.T) {
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  enabled: true
  metrics:
    enabled: true
  agent_logs:
    enabled: false
  detectors:
    bocpd:
      warmup_points: 20
`
	e2e.Run(t, &stdoutReporterSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	))
}

// sendGauge sends one DogStatsD gauge over UDP to the local agent.
// Uses Execute rather than MustExecute: a transient SSH error in the background
// goroutine would otherwise propagate as an unrecovered panic, terminating the
// test process and tearing down the DEV_MODE stack before assertions complete.
func (s *stdoutReporterSuite) sendGauge(name string, value float64) {
	cmd := fmt.Sprintf("bash -c 'echo -n \"%s:%f|g\" > /dev/udp/127.0.0.1/8125'", name, value)
	if _, err := s.Env().RemoteHost.Execute(cmd); err != nil {
		s.T().Logf("sendGauge(%q, %f): SSH error (metric may not have been sent): %v", name, value, err)
	}
}

// TestStdoutReporterEmitsOnDSDSpike sends 60 stable baseline gauges then 30 spike
// gauges (value=5000), expecting BOCPD to fire and the stdout reporter to emit its
// marker within 5 minutes.
func (s *stdoutReporterSuite) TestStdoutReporterEmitsOnDSDSpike() {
	const (
		metricName     = "e2e.anomalydetection.test.gauge"
		baseline       = 1.0
		spike          = 5000.0
		baselinePoints = 60
		spikePoints    = 30
	)

	waitForObserverReady(s)

	// Send baseline and spike in a goroutine so EventuallyWithT can poll concurrently.
	// The deferred cancel+drain guarantees the goroutine exits before the test frame
	// tears down, preventing s.T().Log calls after the test has finished (which panic).
	// A Ticker is used instead of time.After so timers are not leaked when the context
	// is cancelled mid-loop.
	ctx, cancel := context.WithCancel(s.T().Context())
	ticker := time.NewTicker(time.Second)
	done := make(chan struct{})
	defer func() {
		cancel()
		<-done
		ticker.Stop()
	}()
	go func() {
		defer close(done)
		s.T().Logf("sending %d baseline points (value=%.0f)...", baselinePoints, baseline)
		for i := 0; i < baselinePoints; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.sendGauge(metricName, baseline)
			if (i+1)%10 == 0 {
				s.T().Logf("baseline: sent %d/%d", i+1, baselinePoints)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		s.T().Logf("sending %d spike points (value=%.0f)...", spikePoints, spike)
		for i := 0; i < spikePoints; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.sendGauge(metricName, spike)
			if (i+1)%10 == 0 {
				s.T().Logf("spike: sent %d/%d", i+1, spikePoints)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		s.T().Log("done sending metrics")
	}()

	s.T().Log("polling journal for reporter marker...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager -n 10000")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, observerReportMarker, "journald should contain stdout reporter marker")
	}, 5*time.Minute, 5*time.Second)

	dumpObserverLines(s.T(), s.Env())
	s.T().Log("reporter marker found")
}
