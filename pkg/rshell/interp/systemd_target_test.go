// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package interp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithSystemdTargetDefaultsToLocal(t *testing.T) {
	runner, err := New()
	require.NoError(t, err)
	defer runner.Close()

	assert.Equal(t, []string{"/var/log/journal", "/run/log/journal"}, runner.systemdTarget.JournalDirs)
	assert.Equal(t, "/etc/machine-id", runner.systemdTarget.MachineIDPath)
	assert.Equal(t, "/run/systemd/journal/io.systemd.journal", runner.systemdTarget.JournalControlSocket)
	assert.Equal(t, "/run/dbus/system_bus_socket", runner.systemdTarget.ManagerBusSocket)
}

func TestWithSystemdTargetCopiesConfiguration(t *testing.T) {
	dirs := []string{"/host/var/log/journal"}
	runner, err := New(WithSystemdTarget(SystemdTargetConfig{
		JournalDirs:          dirs,
		MachineIDPath:        "/host/etc/machine-id",
		JournalControlSocket: "/host/run/systemd/journal/io.systemd.journal",
		ManagerBusSocket:     "/host/run/dbus/system_bus_socket",
	}))
	require.NoError(t, err)
	defer runner.Close()

	dirs[0] = "/changed"
	assert.Equal(t, []string{"/host/var/log/journal"}, runner.systemdTarget.JournalDirs)
	assert.Equal(t, "/host/run/systemd/journal/io.systemd.journal", runner.systemdTarget.JournalControlSocket)
	assert.Equal(t, "/host/run/dbus/system_bus_socket", runner.systemdTarget.ManagerBusSocket)
}
