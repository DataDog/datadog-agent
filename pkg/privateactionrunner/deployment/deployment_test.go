// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package deployment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		name, goos                                       string
		containerized, processManager, missingSupervisor bool
		want                                             Mode
	}{
		{name: "linux host", goos: "linux", want: Split},
		{name: "linux container", goos: "linux", containerized: true, want: Split},
		{name: "linux host without supervisor", goos: "linux", missingSupervisor: true, want: Monolithic},
		{name: "legacy container without supervisor", goos: "linux", containerized: true, missingSupervisor: true, want: Monolithic},
		{name: "windows host", goos: "windows", processManager: true, want: Split},
		{name: "windows requires process manager", goos: "windows", want: Monolithic},
		{name: "windows container", goos: "windows", processManager: true, containerized: true, want: Monolithic},
		{name: "unsupported host platform", goos: "darwin", want: Monolithic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mock.New(t)
			cfg.SetInTest("private_action_runner.split_enabled", false)
			t.Setenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "false")
			cfg.SetInTest("process_manager.enabled", tc.processManager)
			mode := resolve(cfg, tc.goos, tc.containerized, func(string, bool) bool { return !tc.missingSupervisor })
			require.Equal(t, tc.want, mode)
		})
	}
}

func TestContainerSupervisorUsesSocketWiring(t *testing.T) {
	t.Setenv("DD_INTERNAL_PAR_NO_PROCMGR", "")
	t.Setenv("DD_PM_SOCKET_PATH", "")
	require.False(t, supervisorAvailable("linux", true))

	// The launcher provides the path even before the supervisor starts listening.
	t.Setenv("DD_PM_SOCKET_PATH", filepath.Join(t.TempDir(), "dd-procmgrd.sock"))
	require.True(t, supervisorAvailable("linux", true))
}

func TestHasHostSupervisorRequiresBothPARDefinitions(t *testing.T) {
	installDir := t.TempDir()
	for _, file := range []string{
		"embedded/bin/dd-procmgrd",
		"embedded/bin/par-control",
		"processes.d/datadog-agent-par-control.yaml",
		"processes.d/datadog-agent-action-executor.yml",
	} {
		require.False(t, hasHostSupervisor(installDir, "linux"))
		path := filepath.Join(installDir, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, nil, 0o700))
	}
	require.True(t, hasHostSupervisor(installDir, "linux"))
}
