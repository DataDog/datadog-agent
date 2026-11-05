// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestTheMacOSDaemonIsOptIn pins that the installer daemon does nothing on macOS until the host
// opts in, and that the key it reads is a registered setting.
func TestTheMacOSDaemonIsOptIn(t *testing.T) {
	cfg := configmock.New(t)
	assert.False(t, daemonEnabledOnPlatform(cfg), "the daemon must be off by default on macOS")

	cfg.SetInTest(macOSFleetEnabledKey, true)
	assert.True(t, daemonEnabledOnPlatform(cfg))
}

// TestTheMacOSDaemonNeedsRemoteUpdatesTheOptInAndRemoteConfiguration pins that the daemon runs only
// with all three set.
func TestTheMacOSDaemonNeedsRemoteUpdatesTheOptInAndRemoteConfiguration(t *testing.T) {
	for _, tc := range []struct {
		remoteUpdates, optIn, remoteConfiguration, enabled bool
	}{
		{remoteUpdates: true, optIn: true, remoteConfiguration: true, enabled: true},
		{remoteUpdates: true, optIn: true, remoteConfiguration: false, enabled: false},
		{remoteUpdates: true, optIn: false, remoteConfiguration: true, enabled: false},
		{remoteUpdates: false, optIn: true, remoteConfiguration: true, enabled: false},
		{remoteUpdates: false, optIn: false, remoteConfiguration: true, enabled: false},
	} {
		cfg := configmock.New(t)
		cfg.SetInTest("remote_updates", tc.remoteUpdates)
		cfg.SetInTest(macOSFleetEnabledKey, tc.optIn)
		cfg.SetInTest("remote_configuration.enabled", tc.remoteConfiguration)
		assert.Equal(t, tc.enabled, daemonEnabled(cfg), "remote_updates=%t opt-in=%t remote_configuration=%t", tc.remoteUpdates, tc.optIn, tc.remoteConfiguration)
	}
}

// TestTheMacOSDaemonChecksTheOptInBeforeStarting pins that the check reads datadog.yaml without the
// daemon's components, so a host with remote configuration disabled exits cleanly, opted in or not,
// instead of failing to build the updater.
func TestTheMacOSDaemonChecksTheOptInBeforeStarting(t *testing.T) {
	for _, tc := range []struct {
		name                string
		remoteConfiguration bool
		optIn               bool
		enabled             bool
	}{
		{name: "not opted in, remote configuration disabled", remoteConfiguration: false, optIn: false},
		{name: "opted in, remote configuration disabled", remoteConfiguration: false, optIn: true},
		{name: "opted in", remoteConfiguration: true, optIn: true, enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			config := fmt.Sprintf(`api_key: "0000000000"
remote_configuration:
  enabled: %t
installer:
  log_file: %s
  macos_fleet:
    enabled: %t
`, tc.remoteConfiguration, filepath.Join(dir, "updater.log"), tc.optIn)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(config), 0o600))

			enabled, err := daemonEnabledBeforeStart(&command.GlobalParams{ConfFilePath: filepath.Join(dir, "datadog.yaml")})

			require.NoError(t, err)
			assert.Equal(t, tc.enabled, enabled)
		})
	}
}

// TestDaemonEnabledBeforeStartFxDependencies checks that the pre-start check's Fx graph builds: the
// components it asks for are all provided.
func TestDaemonEnabledBeforeStartFxDependencies(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		_, err := daemonEnabledBeforeStart(&command.GlobalParams{})
		require.NoError(t, err)
	})
}
