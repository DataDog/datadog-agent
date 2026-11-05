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

// TestTheMacOSDaemonRunsByDefault pins that remote_updates defaults to true on macOS, as on other
// platforms, so the daemon runs whenever remote configuration is enabled.
func TestTheMacOSDaemonRunsByDefault(t *testing.T) {
	cfg := configmock.New(t)
	assert.True(t, cfg.GetBool("remote_updates"), "remote_updates must be on by default")

	cfg.SetInTest("remote_configuration.enabled", true)
	assert.True(t, daemonEnabled(cfg))
	cfg.SetInTest("remote_configuration.enabled", false)
	assert.False(t, daemonEnabled(cfg))
}

// TestTheMacOSDaemonNeedsRemoteUpdatesAndRemoteConfiguration pins that the daemon runs only with
// both set.
func TestTheMacOSDaemonNeedsRemoteUpdatesAndRemoteConfiguration(t *testing.T) {
	for _, tc := range []struct {
		remoteUpdates, remoteConfiguration, enabled bool
	}{
		{remoteUpdates: true, remoteConfiguration: true, enabled: true},
		{remoteUpdates: true, remoteConfiguration: false, enabled: false},
		{remoteUpdates: false, remoteConfiguration: true, enabled: false},
		{remoteUpdates: false, remoteConfiguration: false, enabled: false},
	} {
		cfg := configmock.New(t)
		cfg.SetInTest("remote_updates", tc.remoteUpdates)
		cfg.SetInTest("remote_configuration.enabled", tc.remoteConfiguration)
		assert.Equal(t, tc.enabled, daemonEnabled(cfg), "remote_updates=%t remote_configuration=%t", tc.remoteUpdates, tc.remoteConfiguration)
	}
}

// TestTheMacOSDaemonChecksRemoteUpdatesBeforeStarting pins that the check reads datadog.yaml without
// the daemon's components, so a host without remote_updates or with remote configuration disabled
// exits cleanly instead of failing to build the updater.
func TestTheMacOSDaemonChecksRemoteUpdatesBeforeStarting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  string
		enabled bool
	}{
		{name: "remote_updates left at its default", config: "remote_configuration:\n  enabled: true\n", enabled: true},
		{name: "remote_updates disabled", config: "remote_updates: false\nremote_configuration:\n  enabled: true\n"},
		{name: "remote_updates enabled, remote configuration disabled", config: "remote_updates: true\nremote_configuration:\n  enabled: false\n"},
		{name: "remote_updates enabled", config: "remote_updates: true\nremote_configuration:\n  enabled: true\n", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			config := fmt.Sprintf("api_key: \"0000000000\"\ninstaller:\n  log_file: %s\n%s", filepath.Join(dir, "updater.log"), tc.config)
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
