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

//go:embed testdata/errortracking-security-agent-enabled.yaml
var errorTrackingSecurityAgentEnabledConfig string

//go:embed testdata/errortracking-security-agent-disabled.yaml
var errorTrackingSecurityAgentDisabledConfig string

//go:embed testdata/errortracking-security-agent-runtime-security.yaml
var errorTrackingSecurityAgentRuntimeSecurityConfig string

// securityAgentCWSConnectionErrorMessage is the text logged by
// startEventStreamListener (pkg/security/agent/agent.go) whenever the
// security-agent's CWS event-stream client fails to reach the runtime
// security module over its unix socket. Both testdata configs enable
// runtime_security_config without enabling the corresponding system-probe
// module, so nothing ever listens on that socket and this fires
// deterministically on a backoff ticker, without needing any live network
// connection to trigger it.
const securityAgentCWSConnectionErrorMessage = "error while connecting to the runtime security module"

type errorTrackingSecurityAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestErrorTrackingSecurityAgentSuite is the security-agent variant of
// TestAgentTelemetryErrorTrackingSuite: same pkg/util/log/errortracking →
// comp/core/agenttelemetry pipeline, exercised inside the security-agent
// binary instead of the core agent.
func TestErrorTrackingSecurityAgentSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingSecurityAgentSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingSecurityAgentEnabledConfig),
						agentparams.WithSecurityAgentConfig(errorTrackingSecurityAgentRuntimeSecurityConfig),
					),
				),
			),
		),
	)
}

// TestPayloadShape verifies the happy path: the security-agent's own CWS
// connection-error ERROR log reaches FakeIntake with the expected wire
// shape, and — critically for a pipeline shared across many binaries — an
// agent.flavor tag identifying the emitter as security_agent rather than
// agent. Records are filtered by stack trace rather than assumed exclusive,
// since the core agent on the same host shares the same errortracking
// config and could in principle forward its own, unrelated errors during
// the same window.
func (s *errorTrackingSecurityAgentSuite) TestPayloadShape() {
	// testify's suite.Run executes test methods in alphabetical order
	// (TestDisabledByDefault before TestPayloadShape), and UpdateEnv
	// re-provisions the same host in place rather than a fresh one. Re-assert
	// the enabled config here so this test doesn't depend on run order.
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingSecurityAgentEnabledConfig),
				agentparams.WithSecurityAgentConfig(errorTrackingSecurityAgentRuntimeSecurityConfig),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		all, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logs = nil
		for _, l := range all {
			if strings.Contains(l.StackTrace, "pkg/security/agent/agent.go") {
				logs = append(logs, l)
			}
		}
		assert.NotEmpty(c, logs, "no security-agent CWS connection error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for security-agent error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l)
		assert.Contains(s.T(), l.Tags, "agent.flavor:"+flavor.SecurityAgent,
			"tags must identify the security-agent as the emitting binary; got: %q", l.Tags)
	}
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake
// even though the CWS event-stream client keeps failing to connect locally.
func (s *errorTrackingSecurityAgentSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingSecurityAgentDisabledConfig),
				agentparams.WithSecurityAgentConfig(errorTrackingSecurityAgentRuntimeSecurityConfig),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	// Clear the log file after resetting FakeIntake so the wait below only
	// matches an occurrence generated after the reset, not a stale one from
	// before it.
	s.Env().RemoteHost.MustExecute("sudo truncate -s 0 /var/log/datadog/security-agent.log")

	// Wait until the connection error appears in the security-agent's own log
	// file, confirming the error is generated locally before asserting it is
	// not forwarded to telemetry.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, execErr := s.Env().RemoteHost.Execute(
			"sudo grep -cF -- '" + securityAgentCWSConnectionErrorMessage + "' /var/log/datadog/security-agent.log || true")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 1*time.Minute, 5*time.Second, "timed out waiting for connection error to appear in security-agent log")

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it. errortracking is
	// disabled for both the core agent and security-agent here (shared datadog.yaml),
	// so no agent-logs records should arrive from either binary.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
