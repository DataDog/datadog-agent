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

//go:embed testdata/errortracking-system-probe-enabled.yaml
var errorTrackingSystemProbeEnabledConfig string

//go:embed testdata/errortracking-system-probe-disabled.yaml
var errorTrackingSystemProbeDisabledConfig string

//go:embed testdata/errortracking-system-probe-npm.yaml
var errorTrackingSystemProbeNPMConfig string

// systemProbeFilterErrorMessage is the text logged by npcollector's newConfig
// (comp/networkpath/npcollector/impl/config.go) whenever
// network_path.collector.filters fails to unmarshal into []connfilter.Config.
// newConfig runs unconditionally on every system-probe startup, before any
// enabled-check, so a malformed filters value fires this deterministically
// without needing real network activity or a live connections check.
const systemProbeFilterErrorMessage = "Error unmarshalling network_path.collector.filters"

type errorTrackingSystemProbeSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestErrorTrackingSystemProbeSuite is the system-probe variant of
// TestAgentTelemetryErrorTrackingSuite: same pkg/util/log/errortracking →
// comp/core/agenttelemetry pipeline, exercised inside the system-probe binary
// instead of the core agent.
func TestErrorTrackingSystemProbeSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingSystemProbeSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingSystemProbeEnabledConfig),
						agentparams.WithSystemProbeConfig(errorTrackingSystemProbeNPMConfig),
					),
				),
			),
		),
	)
}

// TestPayloadShape verifies the happy path: system-probe's own
// network_path.collector.filters unmarshal-error ERROR log reaches FakeIntake
// with the expected wire shape, and — critically for a pipeline shared across
// many binaries — an agent.flavor tag identifying the emitter as
// system_probe rather than agent. Records are filtered by stack trace rather
// than assumed exclusive, since the core agent on the same host shares the
// same errortracking config and could in principle forward its own, unrelated
// errors during the same window.
func (s *errorTrackingSystemProbeSuite) TestPayloadShape() {
	// Reset FakeIntake before UpdateEnv, not after: the npcollector error is a
	// startup-only trigger with flush_interval_seconds: 1 and no startup
	// jitter, so with the reset placed after UpdateEnv it could delete the
	// only occurrence (delivered during provisioning) before this test ever
	// reads it, leaving EventuallyWithT waiting for a recurrence that never
	// comes.
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	// testify's suite.Run executes test methods in alphabetical order
	// (TestDisabledByDefault before TestPayloadShape), and UpdateEnv
	// re-provisions the same host in place rather than a fresh one. Re-assert
	// the enabled config here so this test doesn't depend on run order; this
	// also restarts system-probe, generating a fresh occurrence of the error
	// after the reset above.
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingSystemProbeEnabledConfig),
				agentparams.WithSystemProbeConfig(errorTrackingSystemProbeNPMConfig),
			),
		),
	))

	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		all, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logs = nil
		for _, l := range all {
			if strings.Contains(l.StackTrace, "comp/networkpath/npcollector/impl/config.go") {
				logs = append(logs, l)
			}
		}
		assert.NotEmpty(c, logs, "no system-probe filter unmarshal error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for system-probe error logs")

	for _, l := range logs {
		assertCommonLogShape(s.T(), l)
		assert.Contains(s.T(), l.Tags, "agent.flavor:"+flavor.SystemProbe,
			"tags must identify system-probe as the emitting binary; got: %q", l.Tags)
	}
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake
// even though the filters unmarshal error fired locally.
func (s *errorTrackingSystemProbeSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(errorTrackingSystemProbeDisabledConfig),
				agentparams.WithSystemProbeConfig(errorTrackingSystemProbeNPMConfig),
			),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	// Unlike the core-agent suite's check-based trigger, the filter unmarshal
	// error fires exactly once, during npcollectorimpl.NewComponent at Fx graph
	// construction — it does not recur on a schedule. By the time UpdateEnv
	// returns, system-probe has already started and the error has already been
	// logged, so confirm it directly rather than truncating and waiting for a
	// recurrence that will never happen.
	out, execErr := s.Env().RemoteHost.Execute(
		"sudo grep -cF -- '" + systemProbeFilterErrorMessage + "' /var/log/datadog/system-probe.log || true")
	require.NoError(s.T(), execErr)
	require.NotEqual(s.T(), "0", strings.TrimSpace(out), "filter unmarshal error must have fired locally")

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it. errortracking is
	// disabled for both the core agent and system-probe here (shared datadog.yaml),
	// so no agent-logs records should arrive from either binary.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
