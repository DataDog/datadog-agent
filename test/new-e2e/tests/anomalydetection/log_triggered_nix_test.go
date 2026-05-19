// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// logTriggeredSuite exercises the log ingestion path of the observer. The agent-log
// tap always forwards Error/Warn/Critical logs (info/debug are sampled at 0 by
// default). A failing http_check generates recurring error logs that flow through
// the log_metrics_extractor, producing a pattern-count metric series that CUSUM
// detects as anomalous.
type logTriggeredSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAnomalyDetectionLogTriggered provisions the agent with an http_check pointed
// at a non-existent endpoint (port 19876) so every check run emits an error log.
func TestAnomalyDetectionLogTriggered(t *testing.T) {
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

// TestLogTriggeredReporterEmitsOnCheckErrors waits for the http_check to produce
// recurring error logs. Those logs flow through the agent-log tap into the
// log_metrics_extractor, which emits a pattern-count metric series that CUSUM
// detects as anomalous.
func (s *logTriggeredSuite) TestLogTriggeredReporterEmitsOnCheckErrors() {
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
	if checkErrors, err := s.Env().RemoteHost.Execute(
		"sudo journalctl -u datadog-agent --no-pager -n 10000 | grep -i 'http_check' || true",
	); err == nil {
		s.T().Logf("http_check lines:\n%s", checkErrors)
	}
	s.T().Log("reporter marker found via log trigger")
}
