// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remoteflags contains E2E tests for features driven by Agent Remote
// Flags, exercised against the Remote Config backend faked by fakeintake.
package remoteflags

import (
	"fmt"
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
)

const (
	// productAgentRemoteFlags is the Remote Config product the Remote Flags
	// client subscribes to.
	productAgentRemoteFlags = "AGENT_REMOTE_FLAGS"

	// flagErrortracking is the remote flag that turns errortracking on.
	flagErrortracking = "agent_errortracking"

	// otherFlag is a flag no component subscribes to.
	otherFlag = "unrelated_flag"

	flagConfigID   = "errortracking_flag"
	flagConfigName = "flags"

	// errorCheckInstance runs the check below every second so that errors keep
	// flowing after the flag lands: Remote Config arrives seconds after start,
	// so the suite never relies on errors logged at boot.
	errorCheckInstance = "init_config:\ninstances:\n  - min_collection_interval: 1\n"

	// errorCheckPy logs an ERROR on every run.
	errorCheckPy = `from datadog_checks.base import AgentCheck


class ErrorCheck(AgentCheck):
    def check(self, instance):
        self.log.error("intentional error for the errortracking remote flag test")
`

	// errortrackingBaseConfig leaves agent_telemetry.errortracking.enabled
	// unset so that only the remote flag can turn the feature on. The short
	// flush interval and no dedup window make every induced error visible
	// within seconds.
	errortrackingBaseConfig = `remote_flags:
  enabled: true
agent_telemetry:
  errortracking:
    flush_interval_seconds: 1
    bouncer_window_seconds: 0
    startup_jitter_seconds: 0
`

	// errortrackingLocalFalseConfig sets the local switch explicitly off.
	errortrackingLocalFalseConfig = `log_level: debug
remote_flags:
  enabled: true
agent_telemetry:
  errortracking:
    enabled: false
    flush_interval_seconds: 1
    bouncer_window_seconds: 0
    startup_jitter_seconds: 0
`
)

type errortrackingFlagSuite struct {
	e2e.BaseSuite[environments.Host]
}

func errortrackingAgentOptions(agentConfig string) []agentparams.Option {
	return []agentparams.Option{
		agentparams.WithAgentConfig(agentConfig),
		agentparams.WithIntegration("error_check.d", errorCheckInstance),
		agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
	}
}

// TestErrortrackingFlagSuite provisions one host whose local errortracking
// setting is unset and drives the feature through the remote flag. The suite
// resets the environment to this provisioner before each test.
//
// Run locally with:
//
//	dda inv new-e2e-tests.run --targets=./tests/remote-flags -run TestErrortrackingFlagSuite
func TestErrortrackingFlagSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &errortrackingFlagSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(errortrackingAgentOptions(errortrackingBaseConfig)...),
				),
			),
		),
	)
}

// errortrackingFlagPayload builds an AGENT_REMOTE_FLAGS payload the way the
// backend does, with the optional override_local some producers add. The
// agent ignores it.
func errortrackingFlagPayload(name string, enabled, overrideLocal bool) []byte {
	if overrideLocal {
		return fmt.Appendf(nil, `{"flags":[{"name":%q,"enabled":%t,"override_local":true}]}`, name, enabled)
	}
	return fmt.Appendf(nil, `{"flags":[{"name":%q,"enabled":%t}]}`, name, enabled)
}

// setFlag pushes (or replaces) the flag configuration.
func (s *errortrackingFlagSuite) setFlag(name string, enabled, overrideLocal bool) {
	s.T().Helper()
	require.NoError(s.T(), s.Env().FakeIntake.Client().RCAddConfig("", productAgentRemoteFlags,
		flagConfigID, flagConfigName, errortrackingFlagPayload(name, enabled, overrideLocal)))
}

// removeFlag deletes every flag configuration, which the agent sees as the
// flag being absent.
func (s *errortrackingFlagSuite) removeFlag() {
	s.T().Helper()
	fi := s.Env().FakeIntake.Client()
	configs, err := fi.RCListConfigs()
	require.NoError(s.T(), err)
	for _, cfg := range configs {
		key := fmt.Sprintf("%s/%s/%s/%s", cfg.OrgID, cfg.Product, cfg.ConfigID, cfg.ConfigName)
		require.NoError(s.T(), fi.RCDeleteConfig(key))
	}
}

// errorEventCount returns the number of errortracking events fakeintake holds.
func (s *errortrackingFlagSuite) errorEventCount() int {
	s.T().Helper()
	logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
	require.NoError(s.T(), err)
	return len(logs)
}

func (s *errortrackingFlagSuite) resetEvents() {
	s.T().Helper()
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}

// requireEvents waits for errortracking events to reach fakeintake.
func (s *errortrackingFlagSuite) requireEvents(msg string) {
	s.T().Helper()
	s.resetEvents()
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		assert.Positive(c, s.errorEventCount(), msg)
	}, 2*time.Minute, 5*time.Second)
}

// requireQuiet waits until a full observation window passes without any new
// event, which tolerates the delay before the agent applies a flag change and
// the events still in flight at that moment, then checks that nothing arrives
// afterwards either.
func (s *errortrackingFlagSuite) requireQuiet(msg string) {
	s.T().Helper()
	const window = 15 * time.Second
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		s.resetEvents()
		time.Sleep(window)
		assert.Zero(c, s.errorEventCount(), msg)
	}, 3*time.Minute, time.Second)

	s.resetEvents()
	assert.Never(s.T(), func() bool { return s.errorEventCount() > 0 }, window, time.Second, msg)
}

func (s *errortrackingFlagSuite) requireAgentReady() {
	s.T().Helper()
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		require.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 5*time.Second, "agent did not become ready")
}

// TestFlagTogglesLive checks the feature follows the flag without a restart:
// off before the flag, on when it is true, off when it is false, on again, and
// off when the flag disappears.
func (s *errortrackingFlagSuite) TestFlagTogglesLive() {
	s.requireAgentReady()
	s.removeFlag()
	s.requireQuiet("no errortracking event is expected before the flag is set")

	s.setFlag(flagErrortracking, true, false)
	s.requireEvents("errortracking events are expected once the flag is true")

	s.setFlag(flagErrortracking, false, false)
	s.requireQuiet("no errortracking event is expected once the flag is false")

	s.setFlag(flagErrortracking, true, false)
	s.requireEvents("errortracking events are expected again once the flag is true again")

	s.removeFlag()
	s.requireQuiet("no errortracking event is expected once the flag is removed")
}

// TestUnrelatedFlagDoesNotEnable checks that other flags leave errortracking off.
func (s *errortrackingFlagSuite) TestUnrelatedFlagDoesNotEnable() {
	s.requireAgentReady()
	s.removeFlag()

	s.setFlag(otherFlag, true, false)
	s.requireQuiet("an unrelated flag must not enable errortracking")
}

// TestLocalFalseWinsOverFlag checks an explicit local false is never overridden,
// even by a flag carrying override_local.
func (s *errortrackingFlagSuite) TestLocalFalseWinsOverFlag() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(errortrackingAgentOptions(errortrackingLocalFalseConfig)...),
		),
	))
	s.requireAgentReady()
	s.removeFlag()

	s.setFlag(flagErrortracking, true, true)

	// Prove the flag reached the agent and was deliberately overridden, so
	// that the quiet period below cannot pass just because it never arrived.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute(
			"sudo grep -c 'errortracking remote flag is true, local setting present: true, errortracking on: false' /var/log/datadog/agent.log || true")
		require.NoError(c, err)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 2*time.Minute, 5*time.Second, "the agent never logged the flag as received")

	s.requireQuiet("an explicit local false must win over the flag")
}
