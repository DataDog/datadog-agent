// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalctlVacuumEndToEnd(t *testing.T) {
	root := t.TempDir()
	machineID := "0123456789abcdef0123456789abcdef"
	machineDir := filepath.Join(root, "var", "log", "journal", machineID)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o700))
	require.NoError(t, os.MkdirAll(machineDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "etc", "machine-id"), []byte(machineID+"\n"), 0o600))
	archive := filepath.Join(machineDir, "system@abcdef0123456789abcdef0123456789-0000000000000001-0000000000000001.journal")
	require.NoError(t, os.WriteFile(archive, make([]byte, 8192), 0o600))
	old := time.Now().Add(-7 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(archive, old, old))

	var stdout, stderr bytes.Buffer
	runner, err := New(
		StdIO(nil, &stdout, &stderr),
		AllowedCommands([]string{"rshell:journalctl"}),
		AllowedSystemServices([]SystemdControlGrant{{
			Service: "systemd-journald.service",
			Actions: []SystemServiceAction{SystemServiceClean},
		}}),
		WithMode(ModeRemediation),
		WithSystemdTarget(SystemdTargetConfig{
			JournalDirs:   []string{filepath.Join(root, "var", "log", "journal")},
			MachineIDPath: filepath.Join(root, "etc", "machine-id"),
		}),
	)
	require.NoError(t, err)
	defer runner.Close()

	program, err := ParseScript("journalctl --vacuum-time=48h", "")
	require.NoError(t, err)
	require.NoError(t, runner.Run(context.Background(), program))
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "Vacuuming done, freed")
	assert.NoFileExists(t, archive)
}
