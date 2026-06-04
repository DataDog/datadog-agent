// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetry contains E2E tests for the agent's internal telemetry
// error-tracking pipeline: pkg/util/log/errortracking → comp/core/agenttelemetry →
// /api/v2/apmtelemetry (request_type: agent-logs).
package agenttelemetry

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed testdata/errortracking-enabled.yaml
var errorTrackingEnabledConfig string

//go:embed testdata/errortracking-disabled.yaml
var errorTrackingDisabledConfig string

//go:embed testdata/error_check.yaml
var errorCheckConfig string

//go:embed testdata/error_check.py
var errorCheckPy string

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
						agentparams.WithIntegration("error_check.d", errorCheckConfig),
						agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
					),
				),
			),
		),
	)
}

// TestPayloadShape verifies the happy path: with errortracking enabled and a
// failing check in place, FakeIntake must receive at least one agent-logs record
// with the expected wire shape.
func (s *errorTrackingSuite) TestPayloadShape() {
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var logs []*aggregator.AgentTelemetryLog
	var err error
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err = s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)
		assert.NotEmpty(c, logs, "no agent-logs telemetry received yet")
	}, 1*time.Minute, 5*time.Second, "timed out waiting for agent-logs telemetry to reach fakeintake")

	for _, l := range logs {
		assert.Equal(s.T(), "ERROR", l.Level)
		assert.NotEmpty(s.T(), l.StackTrace, "stack_trace must be non-empty")
		assert.False(s.T(), l.IsCrash, "agent error logs are not crash reports")
		assert.GreaterOrEqual(s.T(), l.Count, 1)
		// Message is intentionally empty — user-controlled data is never shipped.
		assert.Empty(s.T(), l.Message, "message must not be on the wire")
	}
}

// TestDisabledByDefault verifies that when the errortracking stanza is absent,
// no agent-logs records reach FakeIntake even when errors occur.
func (s *errorTrackingSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingDisabledConfig),
				agentparams.WithIntegration("error_check.d", errorCheckConfig),
				agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
			),
		),
	))
	// flush fakeintake and clear log file
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	_, execErr := s.Env().RemoteHost.Execute("sudo truncate -s 0 /var/log/datadog/agent.log")
	require.NoError(s.T(), execErr)

	// Wait until the check error appears in the agent log — confirming errors are
	// generated locally before asserting they are not forwarded to telemetry.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, execErr := s.Env().RemoteHost.Execute("sudo grep -c 'ERROR.*Error running check' /var/log/datadog/agent.log 2>/dev/null || echo 0")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 1*time.Minute, 5*time.Second, "timed out waiting for check error to appear in agent log")

	// Wait for the error tracking flush interval and confirm nothing arrives.
	// assert.Never polls for 5 s (5× the 1 s flush_interval_seconds) to give the
	// pipeline a full cycle to (incorrectly) forward a payload before declaring success.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
