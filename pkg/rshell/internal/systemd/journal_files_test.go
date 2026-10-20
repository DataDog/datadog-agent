// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMachineIDValidatesAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machine-id")
	require.NoError(t, os.WriteFile(path, []byte("0123456789ABCDEF0123456789ABCDEF\n"), 0o600))

	machineID, err := readMachineID(path)
	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", machineID)

	require.NoError(t, os.WriteFile(path, []byte("not-a-machine-id\n"), 0o600))
	_, err = readMachineID(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly 32 hexadecimal characters")

	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("a", maxMachineIDFileSize+1)), 0o600))
	_, err = readMachineID(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestJournalFilesSelectsRegularFilesForTargetMachine(t *testing.T) {
	root := t.TempDir()
	machineID := "0123456789abcdef0123456789abcdef"
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(machineID+"\n"), 0o600))

	firstBase := filepath.Join(root, "run-journal")
	secondBase := filepath.Join(root, "var-journal")
	firstMachineDir := filepath.Join(firstBase, machineID)
	secondMachineDir := filepath.Join(secondBase, machineID)
	require.NoError(t, os.MkdirAll(firstMachineDir, 0o700))
	require.NoError(t, os.MkdirAll(secondMachineDir, 0o700))

	firstJournal := filepath.Join(firstMachineDir, "system.journal")
	secondJournal := filepath.Join(secondMachineDir, "system@0001.journal")
	require.NoError(t, os.WriteFile(firstJournal, nil, 0o600))
	require.NoError(t, os.WriteFile(secondJournal, nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(firstMachineDir, "ignored.txt"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(firstMachineDir, "directory.journal"), 0o700))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(firstJournal, filepath.Join(firstMachineDir, "link.journal")))
	}

	client := NewClient(Target{
		JournalDirs:   []string{secondBase, firstBase, filepath.Join(root, "missing")},
		MachineIDPath: machineIDPath,
	})
	gotMachineID, files, err := client.journalFiles()
	require.NoError(t, err)
	assert.Equal(t, machineID, gotMachineID)
	assert.Equal(t, []string{firstJournal, secondJournal}, files)
}

func TestJournalFilesRejectsMissingJournalConfiguration(t *testing.T) {
	client := NewClient(Target{MachineIDPath: filepath.Join(t.TempDir(), "machine-id")})
	_, _, err := client.journalFiles()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal directories are not configured")
}

func TestJournalFilesRejectsSymlinkedMachineDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires elevated privileges on Windows")
	}

	root := t.TempDir()
	machineID := "0123456789abcdef0123456789abcdef"
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(machineID+"\n"), 0o600))
	baseDir := filepath.Join(root, "journal")
	outsideDir := filepath.Join(root, "outside")
	require.NoError(t, os.Mkdir(baseDir, 0o700))
	require.NoError(t, os.Mkdir(outsideDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "system.journal"), nil, 0o600))
	require.NoError(t, os.Symlink(outsideDir, filepath.Join(baseDir, machineID)))

	client := NewClient(Target{JournalDirs: []string{baseDir}, MachineIDPath: machineIDPath})
	_, _, err := client.journalFiles()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a real directory")
}
