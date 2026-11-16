// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remoteflags contains E2E tests for features driven by Agent Remote
// Flags, exercised against the Remote Config backend faked by fakeintake.
package remoteflags

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	// productAgentRemoteFlags is the Remote Config product the Remote Flags
	// client subscribes to.
	productAgentRemoteFlags = "AGENT_REMOTE_FLAGS"

	// flagErrortracking is the remote flag that turns errortracking on, by
	// driving errortrackingKey under the Remote Config source.
	flagErrortracking = "agent_errortracking"

	// errortrackingKey is the only setting the feature reads.
	errortrackingKey = "agent_telemetry.errortracking.enabled"

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
	// within seconds. The short inventory intervals make the configuration
	// layers, and therefore the source of the key, reach fakeintake within
	// seconds.
	errortrackingBaseConfig = `remote_flags:
  enabled: true
inventories_min_interval: 1
inventories_max_interval: 10
agent_telemetry:
  errortracking:
    flush_interval_seconds: 1
    bouncer_window_seconds: 0
    startup_jitter_seconds: 0
`

	// errortrackingLocalFalseConfig sets the local switch explicitly off.
	errortrackingLocalFalseConfig = `remote_flags:
  enabled: true
inventories_min_interval: 1
inventories_max_interval: 10
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
// backend does: a name and a value, with no configuration_field. Some
// producers add override_local; the agent ignores it for this flag.
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

// errortrackingConfigValue reads the key from the running agent's
// configuration. The dump leaves out settings still at their default, so an
// absent key reports false and not set.
func (s *errortrackingFlagSuite) errortrackingConfigValue() (value, set bool, err error) {
	out, err := s.Env().Agent.Client.ConfigWithError()
	if err != nil {
		return false, false, err
	}
	var cfg struct {
		AgentTelemetry struct {
			Errortracking struct {
				Enabled *bool `yaml:"enabled"`
			} `yaml:"errortracking"`
		} `yaml:"agent_telemetry"`
	}
	if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
		return false, false, err
	}
	enabled := cfg.AgentTelemetry.Errortracking.Enabled
	if enabled == nil {
		return false, false, nil
	}
	return *enabled, true, nil
}

// requireKeyTrue waits until `agent config` shows the key as true. This proves
// the flag reached the agent and was applied, independently of any event.
func (s *errortrackingFlagSuite) requireKeyTrue(msg string) {
	s.T().Helper()
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		value, set, err := s.errortrackingConfigValue()
		require.NoError(c, err)
		assert.True(c, set && value, msg)
	}, 2*time.Minute, 5*time.Second)
}

// requireKeyNotTrue waits until `agent config` no longer shows the key as true:
// back to the local or default value.
func (s *errortrackingFlagSuite) requireKeyNotTrue(msg string) {
	s.T().Helper()
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		value, _, err := s.errortrackingConfigValue()
		require.NoError(c, err)
		assert.False(c, value, msg)
	}, 2*time.Minute, 5*time.Second)
}

// remoteConfigLayerHasKey reports whether the latest configuration layers sent
// by the agent list the key under the Remote Config source. The key is only
// there while the flag drives it, which is how the source is observed: `agent
// config` only shows the resolved value.
func (s *errortrackingFlagSuite) remoteConfigLayerHasKey() (bool, error) {
	payloads, err := s.Env().FakeIntake.Client().GetMetadata()
	if err != nil {
		return false, err
	}
	var latest *aggregator.MetadataPayload
	for _, p := range payloads {
		if _, ok := p.Metadata["remote_configuration"]; ok && (latest == nil || p.Timestamp > latest.Timestamp) {
			latest = p
		}
	}
	if latest == nil {
		return false, errors.New("no configuration layers received yet")
	}
	layer, _ := latest.Metadata["remote_configuration"].(string)
	var cfg struct {
		AgentTelemetry struct {
			Errortracking struct {
				Enabled *bool `yaml:"enabled"`
			} `yaml:"errortracking"`
		} `yaml:"agent_telemetry"`
	}
	if err := yaml.Unmarshal([]byte(layer), &cfg); err != nil {
		return false, err
	}
	return cfg.AgentTelemetry.Errortracking.Enabled != nil, nil
}

// requireRemoteConfigSource waits until the key is (or is no longer) provided
// by the Remote Config source. The flush does not clear the metadata payloads
// fakeintake keeps, so the latest one by timestamp is used and the retry loop
// absorbs a stale payload.
func (s *errortrackingFlagSuite) requireRemoteConfigSource(want bool, msg string) {
	s.T().Helper()
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		require.NoError(c, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
		time.Sleep(15 * time.Second)
		got, err := s.remoteConfigLayerHasKey()
		require.NoError(c, err)
		assert.Equal(c, want, got, msg)
	}, 5*time.Minute, time.Second)
}

func (s *errortrackingFlagSuite) requireAgentReady() {
	s.T().Helper()
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		require.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 5*time.Second, "agent did not become ready")
}

// TestFlagTogglesLive checks the feature follows the flag without a restart.
// `agent config` shows the key true with the flag, and it goes back to the
// default when the flag is false or removed; events follow the key.
func (s *errortrackingFlagSuite) TestFlagTogglesLive() {
	s.requireAgentReady()
	s.removeFlag()
	s.requireKeyNotTrue("the key must not be set before the flag")
	s.requireQuiet("no errortracking event is expected before the flag is set")

	s.setFlag(flagErrortracking, true, false)
	s.requireKeyTrue("the flag must set the key")
	s.requireRemoteConfigSource(true, "the key must come from the Remote Config source while the flag is true")
	s.requireEvents("errortracking events are expected once the flag is true")

	s.setFlag(flagErrortracking, false, false)
	s.requireKeyNotTrue("the key must be false once the flag is false")
	s.requireQuiet("no errortracking event is expected once the flag is false")

	s.setFlag(flagErrortracking, true, false)
	s.requireKeyTrue("the flag must set the key again")
	s.requireEvents("errortracking events are expected again once the flag is true again")

	s.removeFlag()
	s.requireKeyNotTrue("the key must go back to its default once the flag is removed")
	s.requireRemoteConfigSource(false, "the Remote Config source must give the key back once the flag is removed")
	s.requireQuiet("no errortracking event is expected once the flag is removed")
}

// TestUnrelatedFlagDoesNotEnable checks that other flags leave errortracking off.
func (s *errortrackingFlagSuite) TestUnrelatedFlagDoesNotEnable() {
	s.requireAgentReady()
	s.removeFlag()

	s.setFlag(otherFlag, true, false)
	s.requireKeyNotTrue("an unrelated flag must not set the key")
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

	// Prove the flag reached the agent and was deliberately not applied, so
	// that the quiet period below cannot pass just because it never arrived.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute(
			"sudo grep -c 'not applying to \"" + errortrackingKey + "\"' /var/log/datadog/agent.log || true")
		require.NoError(c, err)
		assert.Regexp(c, `^[1-9][0-9]*$`, strings.TrimSpace(out))
	}, 2*time.Minute, 5*time.Second, "the agent never logged that the flag was not applied")

	value, set, err := s.errortrackingConfigValue()
	require.NoError(s.T(), err)
	assert.True(s.T(), set, "the local setting must still be there")
	assert.False(s.T(), value, "the local false must not be overridden")
	s.requireRemoteConfigSource(false, "the Remote Config source must not provide the key")
	s.requireQuiet("an explicit local false must win over the flag")
}
