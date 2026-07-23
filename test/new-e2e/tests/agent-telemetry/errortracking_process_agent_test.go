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

//go:embed testdata/errortracking-process-agent-enabled.yaml
var errorTrackingProcessAgentEnabledConfig string

//go:embed testdata/errortracking-process-agent-disabled.yaml
var errorTrackingProcessAgentDisabledConfig string

//go:embed testdata/errortracking-process-agent-npm.yaml
var errorTrackingProcessAgentNPMConfig string

// processAgentSubmissionErrorMessage is logged by readResponseStatuses
// (pkg/process/runner/runner.go) whenever the connections check — guaranteed
// to run standalone in process-agent — fails to submit to the
// connection-refused address configured in the testdata above.
const processAgentSubmissionErrorMessage = "[connections] Error from"

type errorTrackingProcessAgentSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestErrorTrackingProcessAgentSuite is the process-agent variant of
// TestAgentTelemetryErrorTrackingSuite: same pkg/util/log/errortracking →
// comp/core/agenttelemetry pipeline, exercised inside the process-agent
// binary instead of the core agent.
func TestErrorTrackingProcessAgentSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingProcessAgentSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingProcessAgentEnabledConfig),
						agentparams.WithSystemProbeConfig(errorTrackingProcessAgentNPMConfig),
					),
				),
			),
		),
	)
}

// TestPayloadShape verifies the process-agent's connections-check
// submission-error log reaches FakeIntake with the expected wire shape,
// tagged agent.flavor:process_agent rather than agent.
func (s *errorTrackingProcessAgentSuite) TestPayloadShape() {
	// BeforeTest already reset the environment to the suite's original
	// (enabled) provisioner regardless of run order, and the connections
	// check's submission error recurs on every check run rather than firing
	// once at construction time, so no re-provisioning is needed here to
	// observe a fresh occurrence after the flush below.
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	assertErrorTrackingLogReceived(s.T(), s.Env(), "pkg/process/runner/runner.go", flavor.ProcessAgent,
		"no process-agent submission error logs received yet")
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake
// even though the connections check keeps failing to submit locally.
func (s *errorTrackingProcessAgentSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingProcessAgentDisabledConfig),
				agentparams.WithSystemProbeConfig(errorTrackingProcessAgentNPMConfig),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	assertNoErrorTrackingWhenDisabled(s.T(), s.Env(), "/var/log/datadog/process-agent.log", processAgentSubmissionErrorMessage,
		"timed out waiting for submission error to appear in process-agent log")
}
