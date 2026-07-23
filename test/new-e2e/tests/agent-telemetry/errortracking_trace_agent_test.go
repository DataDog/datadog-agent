// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed testdata/errortracking-trace-agent-enabled.yaml
var errorTrackingTraceAgentEnabledConfig string

//go:embed testdata/errortracking-trace-agent-disabled.yaml
var errorTrackingTraceAgentDisabledConfig string

// traceAgentReceiverErrorMessage is logged by HTTPReceiver.handleTraces
// (pkg/trace/api/api.go) when a /v0.4/traces request carries a non-numeric
// X-Datadog-Trace-Count header.
const traceAgentReceiverErrorMessage = "Failed to count traces"

type errorTrackingTraceAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestErrorTrackingTraceAgentSuite is the trace-agent variant of
// TestAgentTelemetryErrorTrackingSuite: same pkg/util/log/errortracking →
// comp/core/agenttelemetry pipeline, exercised inside the trace-agent binary
// instead of the core agent.
func TestErrorTrackingTraceAgentSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingTraceAgentSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingTraceAgentEnabledConfig),
					),
				),
			),
		),
	)
}

// triggerReceiverError sends a malformed request to the trace-agent's own
// HTTP receiver, producing one ERROR log, and waits for it to appear in the
// trace-agent's log file before returning.
func (s *errorTrackingTraceAgentSuite) triggerReceiverError() {
	t := s.T()
	_, err := s.Env().RemoteHost.Execute(
		`curl -s -o /dev/null -X POST http://127.0.0.1:8126/v0.4/traces -H "X-Datadog-Trace-Count: notanumber"`)
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, execErr := s.Env().RemoteHost.Execute(
			"sudo awk '/" + traceAgentReceiverErrorMessage + "/{count++} END{print count+0}' /var/log/datadog/trace-agent.log")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 30*time.Second, 2*time.Second, "timed out waiting for receiver error to appear in trace-agent log")
}

// TestPayloadShape verifies the trace-agent's own receiver ERROR log reaches
// FakeIntake with the expected wire shape, tagged agent.flavor:trace_agent
// rather than agent.
func (s *errorTrackingTraceAgentSuite) TestPayloadShape() {
	// testify's suite.Run executes test methods in alphabetical order
	// (TestDisabledByDefault before TestPayloadShape), and UpdateEnv
	// re-provisions the same host in place rather than a fresh one. Re-assert
	// the enabled config here so this test doesn't depend on run order.
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingTraceAgentEnabledConfig),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.triggerReceiverError()

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		all, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logs = nil
		for _, l := range all {
			if strings.Contains(l.StackTrace, "pkg/trace/api/api.go") {
				logs = append(logs, l)
			}
		}
		assert.NotEmpty(c, logs, "no trace-agent receiver error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for trace-agent error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l, flavor.TraceAgent)
	}
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake even
// though the receiver error keeps firing locally.
func (s *errorTrackingTraceAgentSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingTraceAgentDisabledConfig),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.triggerReceiverError()

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it. errortracking is
	// disabled for both the core agent and trace-agent here (shared datadog.yaml),
	// so no agent-logs records should arrive from either binary.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
