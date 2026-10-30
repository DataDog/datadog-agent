// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPARInstallerDefault(t *testing.T) {
	for _, tt := range []struct {
		name, goos, existing, env string
		disabled, fips, noSystemd bool
		want                      *bool
	}{
		{name: "Linux systemd", goos: "linux", want: BoolToPtr(true)},
		{name: "Windows SCM does not require systemd", goos: "windows", noSystemd: true, want: BoolToPtr(true)},
		{name: "Windows YAML opt out", goos: "windows", existing: "private_action_runner:\n  enabled: false\n"},
		{name: "quoted YAML opt out", goos: "linux", existing: "private_action_runner:\n  enabled: 'false'\n"},
		{name: "null setting is explicit", goos: "windows", existing: "private_action_runner:\n  enabled: null\n"},
		{name: "Windows environment opt out", goos: "windows", env: "false", want: BoolToPtr(false)},
		{name: "Linux numeric opt out", goos: "linux", env: "0", want: BoolToPtr(false)},
		{name: "opt out overrides previous default", goos: "windows", env: "FALSE", existing: "private_action_runner:\n  enabled: true\n", want: BoolToPtr(false)},
		{name: "invalid environment is not defaulted", goos: "linux", env: "invalid"},
		{name: "Windows process manager disabled", goos: "windows", disabled: true},
		{name: "Linux process manager disabled", goos: "linux", disabled: true},
		{name: "Linux without systemd", goos: "linux", noSystemd: true},
		{name: "Windows FIPS", goos: "windows", fips: true},
		{name: "Linux FIPS", goos: "linux", fips: true},
		{name: "macOS", goos: "darwin"},
		{name: "other platforms", goos: "aix"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", tt.env)
			path := filepath.Join(t.TempDir(), "datadog.yaml")
			if tt.existing != "" {
				require.NoError(t, os.WriteFile(path, []byte(tt.existing), 0600))
			}
			result, err := PARInstallerDefault(path, tt.goos, !tt.disabled, tt.fips, func() (bool, error) {
				require.Equal(t, "linux", tt.goos, "only Linux should probe systemd")
				return !tt.noSystemd, nil
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestPARInstallerDefaultUTF16(t *testing.T) {
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "")
	path := filepath.Join(t.TempDir(), "datadog.yaml")
	data := []byte{0xff, 0xfe}
	for _, b := range []byte("private_action_runner:\r\n  enabled: false\r\n") {
		data = append(data, b, 0)
	}
	require.NoError(t, os.WriteFile(path, data, 0600))
	enabled, err := PARInstallerDefault(path, "windows", true, false, nil)
	require.NoError(t, err)
	assert.Nil(t, enabled)
}

func TestPARInstallerDefaultProbeError(t *testing.T) {
	enabled, err := PARInstallerDefault("unused", "linux", true, false, func() (bool, error) { return false, errors.New("probe failed") })
	require.ErrorContains(t, err, "probe failed")
	assert.Nil(t, enabled)
}
