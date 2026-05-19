// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package anomalydetection contains e2e tests for the anomaly detection observer component.
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

// stdoutReporterSuite exercises the anomaly detection observer's stdout reporter.
// The test sends a stable DogStatsD gauge baseline followed by a large spike to
// trip the BOCPD detector, then asserts the canonical "[observer] report: pattern="
// marker appears in the agent's systemd journal (stdout → journald by default).
type stdoutReporterSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionStdoutReporter is the entry point for the suite.
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

// sendGauge sends a single DogStatsD gauge over UDP to the local agent.
// Uses Execute rather than MustExecute: a transient SSH error in the background
// goroutine would otherwise propagate as an unrecovered panic, terminating the
// test process and tearing down the DEV_MODE stack before assertions complete.
func (s *stdoutReporterSuite) sendGauge(name string, value float64) {
	cmd := fmt.Sprintf("bash -c 'echo -n \"%s:%f|g\" > /dev/udp/127.0.0.1/8125'", name, value)
	if _, err := s.Env().RemoteHost.Execute(cmd); err != nil {
		s.T().Logf("sendGauge(%q, %f): SSH error (metric may not have been sent): %v", name, value, err)
	}
}

// TestStdoutReporterEmitsOnDSDSpike sends a stable gauge baseline then a spike,
// expecting the BOCPD detector to fire and the stdout reporter to emit its marker.
func (s *stdoutReporterSuite) TestStdoutReporterEmitsOnDSDSpike() {
	const metricName = "e2e.anomalydetection.test.gauge"
	const baseline = 1.0
	const spike = 5000.0
	const baselinePoints = 60
	const spikePoints = 30

	// Wait for the observer to be running before sending metrics.
	// The observer logs "[observer] getting handle for all-metrics" when it is
	// wired into the aggregator — absence means anomaly_detection.enabled is
	// false or the reporter impl is not loaded (e.g. noop build).
	s.T().Log("waiting for observer to be ready...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		assert.NoError(c, err, "reading agent.log to check observer readiness")
		assert.Contains(c, string(out), "[observer] getting handle for all-metrics",
			"agent.log should contain observer startup marker")
	}, 2*time.Minute, 3*time.Second)
	s.T().Log("observer ready")

	// Send baseline and spike in a goroutine so EventuallyWithT can poll concurrently.
	// A done channel drains the goroutine before the test returns, preventing
	// s.T().Log calls after the test has finished (which would panic).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
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
			case <-time.After(time.Second):
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
			case <-time.After(time.Second):
			}
		}
		s.T().Log("done sending metrics")
	}()

	// Poll the systemd journal for the stdout reporter's canonical marker.
	// The reporter prints "[observer] report: pattern=..." to stdout on each
	// advance that yields at least one active correlation.
	s.T().Log("polling journal for [observer] report marker...")
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("sudo journalctl -u datadog-agent --no-pager -n 10000")
		assert.NoError(c, err, "journalctl execution failed")
		assert.Contains(c, out, "[observer] report: pattern=",
			"journald should contain stdout reporter marker")
	}, 5*time.Minute, 5*time.Second)

	// Cancel the sender goroutine and wait for it to exit before the test returns.
	cancel()
	<-done

	// Emit all [observer] lines once at the end for post-mortem diagnosis.
	if observerLines, err := s.Env().RemoteHost.Execute(
		"sudo journalctl -u datadog-agent --no-pager -n 10000 | grep -F '[observer]' || true",
	); err == nil {
		s.T().Logf("observer lines:\n%s", observerLines)
	}
	s.T().Log("[observer] report marker found")
}
