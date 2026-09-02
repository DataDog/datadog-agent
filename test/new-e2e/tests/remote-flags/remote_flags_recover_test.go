// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remoteflags

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These flag names are the contract with the test-only subscriber registered by
// remote_flags.test_subscriber.enabled. They are defined in production as
// comp/core/remoteflags/e2esubscriber.{FeatureFlagName,FaultFlagName}.
const (
	// flagFeature drives the handler that participates in the health monitor /
	// recover lifecycle. Its handler reports healthy unless the fault flag is on.
	flagFeature = "e2e_feature"
	// flagFault flips the shared unhealthy bit, forcing the feature handler
	// unhealthy so the health monitor triggers SafeRecover.
	flagFault = "e2e_fault"
)

// featureFlagConfig builds an AGENT_REMOTE_FLAGS payload for the feature flag,
// bound to a configuration_field and carrying health-monitor tuning. A short
// failures-before-recover makes SafeRecover fire on the first unhealthy tick.
func featureFlagConfig(configField string, enabled bool, version, durationSeconds, failuresBeforeRecover int) []byte {
	return fmt.Appendf(nil,
		`{"flags":[{"name":%q,"enabled":%t,"configuration_field":%q,"override_local":true,"version":%d,"health_check_duration_seconds":%d,"health_check_failures_before_recover":%d}]}`,
		flagFeature, enabled, configField, version, durationSeconds, failuresBeforeRecover,
	)
}

// plainFlagConfig builds an AGENT_REMOTE_FLAGS payload for a flag with no
// configuration_field, used to toggle the fault flag.
func plainFlagConfig(name string, enabled bool, version int) []byte {
	return fmt.Appendf(nil,
		`{"flags":[{"name":%q,"enabled":%t,"version":%d}]}`,
		name, enabled, version,
	)
}

// TestRemoteFlagHealthRecover exercises the FlagHandler recover lifecycle
// end-to-end, which the config-mirroring test does not reach:
//
//   - the feature flag is enabled and mirrors true into logs_enabled;
//     OnChange succeeds and the client starts a health monitor;
//   - the fault flag is enabled, making the feature handler report unhealthy;
//   - the health monitor observes the failure and calls SafeRecover, whose
//     rollback unsets logs_enabled from the RC source.
//
// logs_enabled reverting from true to false is the externally-observable proof
// that OnChange succeeded, the health monitor ran, IsHealthy went false, and
// SafeRecover executed — the whole recover path — without scraping agent logs.
func (s *remoteFlagsSuite) TestRemoteFlagHealthRecover() {
	fi := s.Env().FakeIntake.Client()

	// Start from a clean RC state so a prior test's flags cannot interfere with
	// the version sequencing or the observed configuration field below.
	configs, err := fi.RCListConfigs()
	require.NoError(s.T(), err)
	for _, cfg := range configs {
		// The delete key mirrors the server's storage key:
		// "<org>/<product>/<config_id>/<config_name>".
		key := fmt.Sprintf("%s/%s/%s/%s", cfg.OrgID, cfg.Product, cfg.ConfigID, cfg.ConfigName)
		require.NoError(s.T(), fi.RCDeleteConfig(key))
	}

	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		require.True(c, s.Env().Agent.Client.IsReady())
	}, 2*time.Minute, 5*time.Second, "agent did not become ready")

	// Step 1 — enable the feature. logs_enabled mirrors to true and, because
	// OnChange succeeds, the client begins health-monitoring the handler. A long
	// duration keeps the monitor alive until we inject the fault.
	err = fi.RCAddConfig("", productAgentRemoteFlags, "e2e_recover", "feature",
		featureFlagConfig("logs_enabled", true, 1, 300, 1))
	require.NoError(s.T(), err)

	s.EventuallyWithT(func(c *assert.CollectT) {
		cfg, err := s.Env().Agent.Client.ConfigWithError()
		require.NoError(c, err)
		require.Truef(c, configHas(cfg, "logs_enabled", true),
			"expected logs_enabled to be true after the feature flag was enabled")
	}, 3*time.Minute, 10*time.Second)

	// Step 2 — inject the fault. The feature handler now reports unhealthy; the
	// health monitor calls SafeRecover, which unsets logs_enabled from the RC
	// source, so the value reverts to its default (false).
	err = fi.RCAddConfig("", productAgentRemoteFlags, "e2e_recover_fault", "fault",
		plainFlagConfig(flagFault, true, 1))
	require.NoError(s.T(), err)

	s.EventuallyWithT(func(c *assert.CollectT) {
		cfg, err := s.Env().Agent.Client.ConfigWithError()
		require.NoError(c, err)
		require.Truef(c, configHas(cfg, "logs_enabled", false),
			"expected logs_enabled to revert to false after SafeRecover unset the RC source")
	}, 3*time.Minute, 10*time.Second)
}
