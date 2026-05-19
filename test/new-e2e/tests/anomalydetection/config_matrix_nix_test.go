// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomalydetection

// Item 7 — sub-gate independence matrix (master=on in all cases).
//
// Three independent sub-gates (metrics.enabled, logs.enabled, agent_logs.enabled)
// sit under anomaly_detection.enabled=true. This file verifies that each sub-gate
// activates or suppresses the correct path independently. The master=off case is
// covered by defaults_nix_test.go.
//
// Each case has its own suite type so exactly one test method runs per provisioned
// VM. Sharing a suite type across entry points causes every method on the suite to
// run for every provisioner config, producing cross-contamination failures.

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

// --- Case 1: metrics=off, logs=off ---------------------------------------

type metricsOffLogsOffSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixMetricsOffLogsOff verifies the observer starts but
// emits the deterministic metrics-disabled warning and no agent-logs handle.
func TestObserverConfigMatrixMetricsOffLogsOff(t *testing.T) {
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  enabled: true
  metrics:
    enabled: false
  logs:
    enabled: false
  agent_logs:
    enabled: false
`
	e2e.Run(t, &metricsOffLogsOffSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-both-off"))
}

// TestWarningPresent guards against silent processing when both sub-gates are
// disabled. The metrics-disabled warning (observer.go:243) must appear so
// operators can confirm the expected noop state from logs.
func (s *metricsOffLogsOffSuite) TestWarningPresent() {
	waitForAgentStartup(s)
	time.Sleep(10 * time.Second)

	out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
	assert.NoError(s.T(), err, "reading agent.log")
	agentLog := string(out)
	assert.Contains(s.T(), agentLog, metricsDisabledWarning,
		"metrics-disabled warning should appear when metrics.enabled=false")
	assert.NotContains(s.T(), agentLog, observerAgentLogsMarker,
		"agent-logs handle should not be created when agent_logs.enabled=false")
}

// --- Case 2: metrics=on, logs=off ----------------------------------------

type metricsOnLogsOffSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixMetricsOnLogsOff verifies the metrics path is active
// and the agent-logs tap is not installed.
func TestObserverConfigMatrixMetricsOnLogsOff(t *testing.T) {
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  enabled: true
  metrics:
    enabled: true
  logs:
    enabled: false
  agent_logs:
    enabled: false
`
	e2e.Run(t, &metricsOnLogsOffSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-metrics-on"))
}

// TestSubGateIndependence verifies that enabling the metrics path does not
// silently enable the agent-log tap.
func (s *metricsOnLogsOffSuite) TestSubGateIndependence() {
	waitForObserverReady(s)

	out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
	assert.NoError(s.T(), err, "reading agent.log")
	agentLog := string(out)
	assert.NotContains(s.T(), agentLog, metricsDisabledWarning,
		"no metrics-disabled warning expected when metrics.enabled=true")
	assert.NotContains(s.T(), agentLog, observerAgentLogsMarker,
		"agent-logs handle should not be created when agent_logs.enabled=false")
}

// --- Case 3: metrics=off, logs=on ----------------------------------------

type metricsOffLogsOnSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixMetricsOffLogsOn verifies the agent-log tap is
// installed and the metrics-disabled warning appears.
func TestObserverConfigMatrixMetricsOffLogsOn(t *testing.T) {
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  enabled: true
  metrics:
    enabled: false
  agent_logs:
    enabled: true
`
	e2e.Run(t, &metricsOffLogsOnSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-logs-on"))
}

// TestLogTapActiveMetricsWarningPresent verifies the agent-log tap is installed
// when agent_logs.enabled=true, and that the metrics-disabled warning appears so
// the noop metric path is observable from logs.
func (s *metricsOffLogsOnSuite) TestLogTapActiveMetricsWarningPresent() {
	waitForAgentStartup(s)
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		assert.NoError(c, err, "reading agent.log")
		agentLog := string(out)
		assert.Contains(c, agentLog, metricsDisabledWarning,
			"metrics-disabled warning should appear when metrics.enabled=false")
		assert.Contains(c, agentLog, observerAgentLogsMarker,
			"agent-logs handle should be created when agent_logs.enabled=true")
	}, 2*time.Minute, 3*time.Second)
}

// --- Case 4: all gates on ------------------------------------------------

type allGatesOnSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestObserverConfigMatrixAllGatesOn verifies both the metrics and log paths
// are active simultaneously with no disabled warnings.
func TestObserverConfigMatrixAllGatesOn(t *testing.T) {
	// language=yaml
	agentConfig := `
log_level: debug
anomaly_detection:
  enabled: true
  metrics:
    enabled: true
  agent_logs:
    enabled: true
`
	e2e.Run(t, &allGatesOnSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(scenec2.WithAgentOptions(agentparams.WithAgentConfig(agentConfig))),
		),
	), e2e.WithStackName("anomalydetection-matrix-all-on"))
}

// TestBothPathsActive is the full-active baseline: both handles must be wired
// with no disabled warnings, so a regression disabling either path is immediately
// visible.
func (s *allGatesOnSuite) TestBothPathsActive() {
	waitForObserverReady(s)
	s.EventuallyWithT(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.ReadFilePrivileged("/var/log/datadog/agent.log")
		assert.NoError(c, err, "reading agent.log")
		agentLog := string(out)
		assert.Contains(c, agentLog, observerReadyMarker,
			"metrics path should be active")
		assert.Contains(c, agentLog, observerAgentLogsMarker,
			"agent-logs tap should be installed")
		assert.NotContains(c, agentLog, metricsDisabledWarning,
			"no metrics-disabled warning expected when metrics.enabled=true")
	}, 2*time.Minute, 3*time.Second)
}
