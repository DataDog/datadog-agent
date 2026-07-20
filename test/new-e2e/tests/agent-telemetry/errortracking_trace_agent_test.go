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

// traceAgentRCDisabledErrorMessage is the exact text logged by
// remoteconfighandler.New (pkg/trace/remoteconfighandler/remote_config_handler.go)
// when the debug server is disabled and no other RC product is enabled. Unlike
// the core/cluster agents, trace-agent has no periodic check loop to reuse as an
// error source, so this once-at-startup, config-triggered error is used instead;
// the tests below restart the trace-agent service to get a fresh occurrence on
// demand rather than waiting on a repeating timer.
const traceAgentRCDisabledErrorMessage = "RC is disabled"

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

// restartTraceAgentFresh clears the trace-agent log file and restarts the
// trace-agent's own systemd unit (independent of the core agent) so the
// once-at-startup RC-disabled error fires again, deterministically, right
// after the FakeIntake reset the caller is expected to have already done.
func (s *errorTrackingTraceAgentSuite) restartTraceAgentFresh() {
	t := s.T()
	_, err := s.Env().RemoteHost.Execute("sudo truncate -s 0 /var/log/datadog/trace-agent.log")
	require.NoError(t, err)
	_, err = s.Env().RemoteHost.Execute("sudo systemctl restart datadog-agent-trace.service")
	require.NoError(t, err)

	// Wait until the RC-disabled error appears in the trace-agent's own log
	// file, confirming the error is generated locally before asserting
	// anything about whether it was forwarded.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, execErr := s.Env().RemoteHost.Execute(
			"sudo awk '/" + traceAgentRCDisabledErrorMessage + "/{count++} END{print count+0}' /var/log/datadog/trace-agent.log")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 1*time.Minute, 5*time.Second, "timed out waiting for RC-disabled error to appear in trace-agent log")
}

// TestPayloadShape verifies the happy path: the trace-agent's own RC-disabled
// ERROR log reaches FakeIntake with the expected wire shape, and — critically
// for a pipeline shared across many binaries — an agent.flavor tag identifying
// the emitter as trace_agent rather than agent. Records are filtered by stack
// trace rather than assumed exclusive, since the core agent on the same host
// shares the same errortracking config and could in principle forward its own,
// unrelated errors during the same window.
func (s *errorTrackingTraceAgentSuite) TestPayloadShape() {
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.restartTraceAgentFresh()

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		all, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logs = nil
		for _, l := range all {
			if strings.Contains(l.StackTrace, "remote_config_handler.go") {
				logs = append(logs, l)
			}
		}
		assert.NotEmpty(c, logs, "no trace-agent RC-disabled error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for trace-agent error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l)
		assert.Contains(s.T(), l.Tags, "agent.flavor:"+flavor.TraceAgent,
			"tags must identify the trace-agent as the emitting binary; got: %q", l.Tags)
	}
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake even
// though the RC-disabled error keeps firing locally.
func (s *errorTrackingTraceAgentSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingTraceAgentDisabledConfig),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	s.restartTraceAgentFresh()

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
