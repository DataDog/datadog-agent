// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetry

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

//go:embed testdata/errortracking-security-agent-enabled.yaml
var errorTrackingSecurityAgentEnabledConfig string

//go:embed testdata/errortracking-security-agent-disabled.yaml
var errorTrackingSecurityAgentDisabledConfig string

//go:embed testdata/errortracking-security-agent-runtime-security.yaml
var errorTrackingSecurityAgentRuntimeSecurityConfig string

// securityAgentCWSConnectionErrorMessage is logged by startEventStreamListener
// (pkg/security/agent/agent.go) whenever the CWS event-stream client fails to
// reach the runtime security module, which the testdata configs never enable.
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

// TestPayloadShape verifies the security-agent's CWS connection-error log
// reaches FakeIntake with the expected wire shape, tagged
// agent.flavor:security_agent rather than agent.
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

	assertErrorTrackingLogReceived(s.T(), s.Env(), "pkg/security/agent/agent.go", flavor.SecurityAgent,
		"no security-agent CWS connection error logs received yet")
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

	assertNoErrorTrackingWhenDisabled(s.T(), s.Env(), "/var/log/datadog/security-agent.log", securityAgentCWSConnectionErrorMessage,
		"timed out waiting for connection error to appear in security-agent log")
}
