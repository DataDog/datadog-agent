// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package anomalydetection contains E2E tests for the anomaly detection observer.
// Each file in this package covers one concern:
//
//   - anomalydetection_nix_test.go — reporter tests: DSD-spike (BOCPD) and log-triggered (CUSUM)
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

// logTriggeredSuite exercises the log ingestion path of the observer. The agent-log
// tap always forwards Error/Warn/Critical logs (info/debug are sampled at 0 by
// default). A failing http_check generates recurring error logs that flow through
// the log_metrics_extractor, producing a pattern-count metric series that CUSUM
// detects as anomalous.
type logTriggeredSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionLogsTriggered provisions the agent with an http_check pointed
// at a non-existent endpoint (port 19876) so every check run emits an error log.
func TestAnomalyDetectionLogsTriggered(t *testing.T) {
	// language=yaml
	checkConfig := `
init_config:
instances:
  - name: anomaly-detection-e2e-target
    url: http://127.0.0.1:19876/
    timeout: 2
    min_collection_interval: 10
`
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
    cusum:
      enabled: true
    bocpd:
      enabled: false
`
	e2e.Run(t, &logTriggeredSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfig),
				agentparams.WithIntegration("http_check.d", checkConfig),
			)),
		),
	), e2e.WithStackName("anomalydetection-log-triggered"))
}

// TestLogsTriggeredEmitsOnCheckErrors waits for the http_check to produce recurring
// error logs. Those logs flow through the agent-log tap into the log_metrics_extractor,
// which emits a pattern-count metric series that CUSUM detects as anomalous.
func (s *logTriggeredSuite) TestLogsTriggeredEmitsOnCheckErrors() {
	// http_check runs every 10 s and emits an error log (error level → always forwarded).
	// CUSUM fires after min_points=5 (default); the first occurrences look like a spike
	// against an empty/zero baseline.
	waitForObserverReady(s)

	s.T().Log("waiting for [observer] report marker from log-triggered anomaly...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, observerReportMarker, "journald should contain stdout reporter marker")
	}, 5*time.Minute, 10*time.Second)

	dumpObserverLines(s.T(), s.Env())
	if out, err := s.Env().RemoteHost.Execute(
		"sudo journalctl -u datadog-agent --no-pager | grep -i 'http_check' || true",
	); err == nil {
		s.T().Logf("http_check lines:\n%s", out)
	}
	s.T().Log("reporter marker found via log trigger")
}
