// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package systemd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTargetDefaultsToLocalPaths(t *testing.T) {
	target, err := ResolveTarget(Target{})
	require.NoError(t, err)

	assert.Equal(t, []string{"/var/log/journal", "/run/log/journal"}, target.JournalDirs)
	assert.Equal(t, "/etc/machine-id", target.MachineIDPath)
	assert.Equal(t, "/run/systemd/journal/io.systemd.journal", target.JournalControlSocket)
	assert.Equal(t, "/run/dbus/system_bus_socket", target.ManagerBusSocket)
}

func TestResolveTargetUsesOnlyExplicitPaths(t *testing.T) {
	target, err := ResolveTarget(Target{
		JournalDirs:          []string{"/mnt/logs", "/mnt/logs", "/mnt/runtime-logs/../runtime-logs"},
		MachineIDPath:        "/mnt/etc/machine-id",
		JournalControlSocket: "/mnt/run/journal.sock",
		ManagerBusSocket:     "/mnt/run/dbus/system_bus_socket",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"/mnt/logs", "/mnt/runtime-logs"}, target.JournalDirs)
	assert.Equal(t, "/mnt/etc/machine-id", target.MachineIDPath)
	assert.Equal(t, "/mnt/run/journal.sock", target.JournalControlSocket)
	assert.Equal(t, "/mnt/run/dbus/system_bus_socket", target.ManagerBusSocket)
}

func TestResolveTargetManagerOnlyDoesNotFallBackToLocalPaths(t *testing.T) {
	target, err := ResolveTarget(Target{
		MachineIDPath:    "/host/etc/machine-id",
		ManagerBusSocket: "/host/run/dbus/system_bus_socket",
	})
	require.NoError(t, err)

	assert.Empty(t, target.JournalDirs)
	assert.Empty(t, target.JournalControlSocket)
	assert.Equal(t, "/host/etc/machine-id", target.MachineIDPath)
	assert.Equal(t, "/host/run/dbus/system_bus_socket", target.ManagerBusSocket)
}

func TestResolveTargetRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		target Target
		needle string
	}{
		{
			name:   "explicit without machine ID",
			target: Target{JournalDirs: []string{"/host/var/log/journal"}},
			needle: "machine ID path is required",
		},
		{
			name:   "relative journal directory",
			target: Target{JournalDirs: []string{"logs"}, MachineIDPath: "/etc/machine-id"},
			needle: "must be absolute",
		},
		{
			name:   "relative journal control socket",
			target: Target{MachineIDPath: "/etc/machine-id", JournalControlSocket: "run/systemd/journal.sock"},
			needle: "must be absolute",
		},
		{
			name:   "relative manager bus socket",
			target: Target{MachineIDPath: "/etc/machine-id", ManagerBusSocket: "run/dbus/system_bus_socket"},
			needle: "must be absolute",
		},
		{
			name:   "NUL path",
			target: Target{MachineIDPath: "/etc/machine-id\x00suffix"},
			needle: "NUL byte",
		},
		{
			name: "too many journal directories",
			target: Target{
				JournalDirs:   []string{"/1", "/2", "/3", "/4", "/5", "/6", "/7", "/8", "/9"},
				MachineIDPath: "/etc/machine-id",
			},
			needle: "maximum is 8",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolveTarget(test.target)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.needle)
		})
	}
}
