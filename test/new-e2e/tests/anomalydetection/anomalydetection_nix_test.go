// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package anomalydetection contains E2E tests for the anomaly detection observer.
// Each file in this package covers one concern:
//
//   - anomalydetection_nix_test.go — reporter tests: DSD-spike (CUSUM) and file-log-spike (BOCPD)
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

// metricsTriggeredSuite exercises the DSD-metrics path of the observer. It sends a
// stable gauge baseline followed by a large spike to trip the BOCPD detector, then
// asserts the canonical "[observer] report: pattern=" marker appears in the agent's
// systemd journal (stdout → journald by default).
type metricsTriggeredSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionMetricsTriggered provisions a Linux VM with the observer
// enabled using the CUSUM detector.
//
// CUSUM is preferred over BOCPD here because:
//   - It fires deterministically after min_points=5 data points (default).
//   - It does not require a long warmup phase (BOCPD default: 120 points).
//   - With a constant baseline the stddev≈0 path sets threshold=10%×mean,
//     so even a small spike fires immediately.
//
// dogstatsd_flush_interval=1 ensures each UDP send produces a distinct
// one-second storage point in the observer, rather than being aggregated
// into a single 10-second DSD flush bucket.
func TestAnomalyDetectionMetricsTriggered(t *testing.T) {
	// language=yaml
	agentConfig := `
log_level: debug
dogstatsd_flush_interval: 1
anomaly_detection:
  enabled: true
  metrics:
    enabled: true
  logs:
    enabled: false
  agent_logs:
    enabled: false
  detectors:
    cusum:
      enabled: true
    bocpd:
      enabled: false
`
	e2e.Run(t, &metricsTriggeredSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	))
}

// sendGauge sends one DogStatsD gauge over UDP to the local agent.
// Uses Execute rather than MustExecute: a transient SSH error in the background
// goroutine would otherwise propagate as an unrecovered panic, terminating the
// test process and tearing down the DEV_MODE stack before assertions complete.
func (s *metricsTriggeredSuite) sendGauge(name string, value float64) {
	cmd := fmt.Sprintf("bash -c 'echo -n \"%s:%f|g\" > /dev/udp/127.0.0.1/8125'", name, value)
	if _, err := s.Env().RemoteHost.Execute(cmd); err != nil {
		s.T().Logf("sendGauge(%q, %f): SSH error (metric may not have been sent): %v", name, value, err)
	}
}

// TestMetricsTriggeredEmitsOnDSDSpike sends a stable gauge baseline then a large
// spike, expecting CUSUM to fire and the stdout reporter to emit its marker.
//
// Point counts: 15 baseline (well above the 5-point CUSUM minimum) followed by
// 10 spike points — total ~25 seconds of data. The spike is 5000× the baseline
// so CUSUM's stddev-based threshold fires on the very first spike point.
func (s *metricsTriggeredSuite) TestMetricsTriggeredEmitsOnDSDSpike() {
	const (
		metricName     = "e2e.anomalydetection.test.gauge"
		baseline       = 1.0
		spike          = 5000.0
		baselinePoints = 15
		spikePoints    = 10
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

	// Poll the journal for the reporter marker. The stdoutReporter writes via
	// fmt.Printf (→ process stdout → journald). No line cap is applied so we
	// never miss the marker because of journal truncation.
	s.T().Log("polling journal for reporter marker...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, observerReportMarker, "journald should contain stdout reporter marker")
	}, 3*time.Minute, 5*time.Second)

	dumpObserverLines(s.T(), s.Env())
	s.T().Log("reporter marker found")
}

// logTriggeredSuite exercises the external log collection path of the observer.
// The test writes plain-text log lines to a file that the agent tails via the
// logssource pipeline. The external path has no level/status filter — every line
// reaches log_metrics_extractor, which emits a pattern-count metric series that
// BOCPD detects as anomalous on a frequency spike.
type logTriggeredSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionLogsTriggered provisions the agent with a file-tailing log
// integration. The test itself writes the signal: a stable baseline of one line per
// second (BOCPD warmup), then a burst of 20 lines in rapid succession (spike).
//
// The external logssource path (logs.enabled) has no level/status filter, so plain
// text lines are sufficient — no JSON or ERROR status required.
func TestAnomalyDetectionLogsTriggered(t *testing.T) {
	// language=yaml
	logConfig := `
logs:
  - type: file
    path: /tmp/e2e-anomaly-test.log
    service: e2e-anomaly
    source: e2e
`
	// language=yaml
	agentConfig := `
log_level: debug
logs_enabled: true
anomaly_detection:
  enabled: true
  metrics:
    enabled: false
  logs:
    enabled: true
  agent_logs:
    enabled: false
  detectors:
    bocpd:
      warmup_points: 20
`
	e2e.Run(t, &logTriggeredSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfig),
				agentparams.WithIntegration("custom_logs.d", logConfig),
			)),
		),
	), e2e.WithStackName("anomalydetection-log-triggered"))
}

// writeLogLine appends one line to the tailed log file on the remote host.
// Uses Execute (not MustExecute) so a transient SSH error in the background
// goroutine does not panic and tear down a DEV_MODE stack.
func (s *logTriggeredSuite) writeLogLine(msg string) {
	cmd := fmt.Sprintf("echo %q >> /tmp/e2e-anomaly-test.log", msg)
	if _, err := s.Env().RemoteHost.Execute(cmd); err != nil {
		s.T().Logf("writeLogLine(%q): SSH error: %v", msg, err)
	}
}

// TestLogsTriggeredEmitsOnFileSpike writes a stable baseline of log lines (one per
// second, satisfying BOCPD warmup_points=20), then bursts 20 lines in rapid
// succession. The frequency step-change causes BOCPD to fire, and the stdout
// reporter emits its canonical marker.
//
// Timing: ~25 s baseline + ~instant burst = detection within ~30 s of spike.
func (s *logTriggeredSuite) TestLogsTriggeredEmitsOnFileSpike() {
	const (
		baselineLines = 25
		spikeLines    = 20
	)

	// Create the file before the tailer starts so the agent picks it up immediately.
	s.Env().RemoteHost.MustExecute("touch /tmp/e2e-anomaly-test.log")

	waitForObserverReady(s)

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
		s.T().Logf("writing %d baseline log lines (1/s)...", baselineLines)
		for i := 0; i < baselineLines; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.writeLogLine(fmt.Sprintf("e2e anomaly baseline %d", i))
			if (i+1)%10 == 0 {
				s.T().Logf("baseline: wrote %d/%d", i+1, baselineLines)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		s.T().Logf("writing %d spike log lines (burst)...", spikeLines)
		for i := 0; i < spikeLines; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.writeLogLine(fmt.Sprintf("e2e anomaly spike %d", i))
		}
		s.T().Log("done writing log lines")
	}()

	s.T().Log("polling journal for reporter marker...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, observerReportMarker, "journald should contain stdout reporter marker")
	}, 3*time.Minute, 5*time.Second)

	dumpObserverLines(s.T(), s.Env())
	s.T().Log("reporter marker found via log trigger")
}
