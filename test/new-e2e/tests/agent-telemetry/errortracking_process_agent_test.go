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

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		all, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logs = nil
		for _, l := range all {
			if strings.Contains(l.StackTrace, "pkg/process/runner/runner.go") {
				logs = append(logs, l)
			}
		}
		assert.NotEmpty(c, logs, "no process-agent submission error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for process-agent error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l, flavor.ProcessAgent)
	}
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

	// Clear the log file after resetting FakeIntake so the wait below only
	// matches an occurrence generated after the reset, not a stale one from
	// before it.
	s.Env().RemoteHost.MustExecute("sudo truncate -s 0 /var/log/datadog/process-agent.log")

	// Wait until the submission error appears in the process-agent's own log
	// file, confirming the error is generated locally before asserting it is
	// not forwarded to telemetry.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, execErr := s.Env().RemoteHost.Execute(
			"sudo grep -cF -- '" + processAgentSubmissionErrorMessage + "' /var/log/datadog/process-agent.log || true")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 2*time.Minute, 5*time.Second, "timed out waiting for submission error to appear in process-agent log")

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it. errortracking is
	// disabled for both the core agent and process-agent here (shared datadog.yaml),
	// so no agent-logs records should arrive from either binary.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
