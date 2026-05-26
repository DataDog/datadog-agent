// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetry contains E2E tests for the agent's internal telemetry
// error-tracking pipeline: pkg/util/log/errortracking → comp/core/agenttelemetry →
// /api/v2/apmtelemetry (request_type: agent-logs).
package agenttelemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	waitFor = 2 * time.Minute
	tick    = 10 * time.Second
)

// errorTrackingEnabledConfig enables the error-tracking feature with a short
// flush interval and zero jitter/bouncer window for deterministic test behaviour.
const errorTrackingEnabledConfig = `
agent_telemetry:
  enabled: true
  errortracking:
    enabled: true
    flush_interval_seconds: 10
    bouncer_window_seconds: 0
    startup_jitter_seconds: 0
`

// errorTrackingDisabledConfig omits the errortracking stanza entirely,
// verifying that the feature is off by default.
const errorTrackingDisabledConfig = `
agent_telemetry:
  enabled: true
`

// failingCheckConfig points http_check at a guaranteed-closed local port.
// The Go check runner logs at Error via pkg/util/log when the TCP connection
// is refused, which flows through the slog errortracking handler into errLogsCh.
const failingCheckConfig = `
init_config:
instances:
  - name: test-unreachable
    url: http://127.0.0.1:19998/
    timeout: 1
`

type errorTrackingSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAgentTelemetryErrorTrackingSuite is the entry point for the suite.
func TestAgentTelemetryErrorTrackingSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingEnabledConfig),
						agentparams.WithIntegration("http_check.d", failingCheckConfig),
					),
				),
			),
		),
		e2e.WithStackName("agent-telemetry-errortracking"),
	)
}

// BeforeTest flushes both the server and all client aggregators before each
// test case so payload state from a previous test cannot bleed in.
func (s *errorTrackingSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

// TearDownSuite flushes on teardown (best-effort) before calling the base
// implementation.
func (s *errorTrackingSuite) TearDownSuite() {
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators() //nolint:errcheck
	s.BaseSuite.TearDownSuite()
}

// TestPayloadShape verifies the happy path: with errortracking enabled and a
// failing check in place, FakeIntake must receive at least one agent-logs record
// with the expected wire shape.
func (s *errorTrackingSuite) TestPayloadShape() {
	// Phase 1: poll until at least one record arrives.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		assert.NoError(c, err)
		assert.NotEmpty(c, logs, "no agent-logs telemetry received yet")
	}, waitFor, tick, "timed out waiting for agent-logs telemetry to reach fakeintake")

	// Phase 2: assert wire shape outside the polling loop.
	logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), logs)

	l := logs[0]
	assert.Equal(s.T(), "ERROR", l.Level)
	assert.NotEmpty(s.T(), l.StackTrace, "stack_trace must be non-empty")
	assert.False(s.T(), l.IsCrash, "agent error logs are not crash reports")
	assert.GreaterOrEqual(s.T(), l.Count, 1)
	// Message is intentionally empty — user-controlled data is never shipped.
	assert.Empty(s.T(), l.Message, "message must not be on the wire")
}

// TestDisabledByDefault verifies that when the errortracking stanza is absent,
// no agent-logs records reach FakeIntake even when errors occur.
func (s *errorTrackingSuite) TestDisabledByDefault() {
	s.Run("reconfigure without errortracking", func() {
		s.UpdateEnv(awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(errorTrackingDisabledConfig),
					agentparams.WithIntegration("http_check.d", failingCheckConfig),
				),
			),
		))
		require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

		// Wait longer than two flush intervals to give a misconfigured agent
		// time to send if the gate were broken.
		time.Sleep(30 * time.Second)

		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		assert.Empty(s.T(), logs, "no agent-logs must arrive when errortracking is disabled")
	})
}
