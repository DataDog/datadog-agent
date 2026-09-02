// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteflags contains E2E tests for the Agent Remote Flags feature
// (comp/core/remoteflags), exercised end-to-end against the Remote Config
// backend faked by fakeintake.
package remoteflags

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// productAgentRemoteFlags is the Remote Config product the Remote Flags client
// subscribes to. It is the RC backend contract, defined in production as
// pkg/config/remote/data.ProductAgentFlags / pkg/remoteconfig/state.ProductAgentFlags.
const productAgentRemoteFlags = "AGENT_REMOTE_FLAGS"

type remoteFlagsSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestRemoteFlagsSuite is the entry point for the Remote Flags E2E suite.
//
// Run locally with:
//
//	dda inv new-e2e-tests.run --targets=./tests/remote-flags -run TestRemoteFlagsSuite
func TestRemoteFlagsSuite(t *testing.T) {
	t.Parallel()
	// Remote Config is wired automatically by the fakeintake-backed
	// provisioner; we only need to enable the Remote Flags feature.
	//
	// remote_flags.test_subscriber.enabled registers the test-only subscriber
	// (comp/core/remoteflags/e2esubscriber) so the handler lifecycle
	// (OnChange/IsHealthy/SafeRecover) can be exercised end-to-end. It is inert
	// for the mirror test, which pushes flags no handler subscribes to.
	e2e.Run(t, &remoteFlagsSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					scenec2.WithAgentOptions(
						agentparams.WithAgentConfig(
							"remote_flags.enabled: true\nremote_flags.test_subscriber.enabled: true",
						),
					),
				),
			),
		),
	)
}

// flagConfig builds the JSON body of an AGENT_REMOTE_FLAGS Remote Config
// payload holding a single flag bound to a configuration field. The structure
// matches pkg/remoteflags.FlagConfig.
func flagConfig(name, configField string, enabled bool, version int) []byte {
	return fmt.Appendf(nil,
		`{"flags":[{"name":%q,"enabled":%t,"configuration_field":%q,"override_local":true,"version":%d}]}`,
		name, enabled, configField, version,
	)
}

// configHas reports whether the agent's runtime configuration dump contains the
// exact `key: value` line, i.e. the setting resolved to that boolean value.
func configHas(cfg, key string, value bool) bool {
	return strings.Contains(cfg, fmt.Sprintf("%s: %t", key, value))
}

// TestRemoteFlagMirrorsToConfig verifies the full Remote Flags pipeline:
// Remote Config delivers an AGENT_REMOTE_FLAGS payload, the Remote Flags client
// parses it and mirrors a flag bound to a configuration_field into the running
// agent configuration under the RC source. It also exercises flag versioning by
// pushing a newer version that flips the value back.
//
// The flag is bound to `logs_enabled` (a boolean, default false) purely as an
// observable config field: the assertion is on the resolved configuration
// value, which proves the RC -> Remote Flags -> pkg/config path end-to-end.
func (s *remoteFlagsSuite) TestRemoteFlagMirrorsToConfig() {
	fi := s.Env().FakeIntake.Client()

	// Step 1 — the agent is up and logs_enabled is still at its default (false).
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		require.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 5*time.Second, "agent did not become ready")

	cfg, err := s.Env().Agent.Client.ConfigWithError()
	require.NoError(s.T(), err)
	require.Truef(s.T(), configHas(cfg, "logs_enabled", false),
		"expected logs_enabled to default to false, got:\n%s", cfg)

	// Step 2 — push a flag (version 1) that enables logs_enabled via RC.
	err = fi.RCAddConfig("", productAgentRemoteFlags, "e2e_flags", "logs_flag",
		flagConfig("e2e_logs_enabled", "logs_enabled", true, 1))
	require.NoError(s.T(), err)

	// The agent polls RC every 5s (remote_configuration.refresh_interval).
	s.EventuallyWithT(func(c *assert.CollectT) {
		cfg, err := s.Env().Agent.Client.ConfigWithError()
		require.NoError(c, err)
		require.Truef(c, configHas(cfg, "logs_enabled", true),
			"expected logs_enabled to be true after remote flag v1")
	}, 3*time.Minute, 10*time.Second)

	// Step 3 — push a newer flag (version 2) that flips the value back, proving
	// version sequencing applies the strictly-greater version.
	err = fi.RCAddConfig("", productAgentRemoteFlags, "e2e_flags", "logs_flag",
		flagConfig("e2e_logs_enabled", "logs_enabled", false, 2))
	require.NoError(s.T(), err)

	s.EventuallyWithT(func(c *assert.CollectT) {
		cfg, err := s.Env().Agent.Client.ConfigWithError()
		require.NoError(c, err)
		require.Truef(c, configHas(cfg, "logs_enabled", false),
			"expected logs_enabled to be false after remote flag v2")
	}, 3*time.Minute, 10*time.Second)
}
