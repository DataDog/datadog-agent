// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package anomalydetection contains e2e tests for the anomaly detection observer component.
package anomalydetection

import (
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
// trip the CUSUM detector, then asserts the canonical "[observer] report: pattern="
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
  agent_logs:
    enabled: false
  detectors:
    cusum:
      enabled: true
    bocpd:
      enabled: false
`
	e2e.Run(t, &stdoutReporterSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	))
}

// sendGauge sends a single DogStatsD gauge over UDP to the local agent.
func (s *stdoutReporterSuite) sendGauge(name string, value float64) {
	cmd := fmt.Sprintf("bash -c 'echo -n \"%s:%f|g\" > /dev/udp/127.0.0.1/8125'", name, value)
	s.Env().RemoteHost.MustExecute(cmd)
}

// TestStdoutReporterEmitsOnDSDSpike sends a stable gauge baseline then a spike,
// expecting the CUSUM detector to fire and the stdout reporter to emit its marker.
func (s *stdoutReporterSuite) TestStdoutReporterEmitsOnDSDSpike() {
	const metricName = "e2e.anomalydetection.test.gauge"
	const baseline = 1.0
	const spike = 5000.0
	const baselinePoints = 20
	const spikePoints = 10

	// Send baseline and spike in the background so EventuallyWithT can poll concurrently.
	go func() {
		for i := 0; i < baselinePoints; i++ {
			s.sendGauge(metricName, baseline)
			time.Sleep(1 * time.Second)
		}
		for i := 0; i < spikePoints; i++ {
			s.sendGauge(metricName, spike)
			time.Sleep(1 * time.Second)
		}
	}()

	// Poll the systemd journal for the stdout reporter's canonical marker.
	// The reporter prints "[observer] report: pattern=..." to stdout on each
	// advance that yields at least one active correlation.
	s.EventuallyWithT(func(c *assert.CollectT) {
		out := s.Env().RemoteHost.MustExecute("sudo journalctl -u datadog-agent --no-pager -n 10000")
		assert.Contains(c, out, "[observer] report: pattern=")
	}, 3*time.Minute, 5*time.Second)
}
