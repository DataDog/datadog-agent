// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalSnapshotDetectsFileReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit replacing this open fixture consistently")
	}

	client, journalDir, machineID := newJournalSnapshotTestClient(t)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: repeatedJournalID(0x22), realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=ready"}},
	})
	path := writeJournalSnapshotFixture(t, journalDir, "system.journal", contents, machineID, repeatedJournalID(0x31), repeatedJournalID(0x44))
	replacement, err := os.ReadFile(path)
	require.NoError(t, err)

	snapshot, err := client.openJournalSnapshot()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.close()) })
	require.NoError(t, os.Rename(path, path+".replaced"))
	require.NoError(t, os.WriteFile(path, replacement, 0o600))

	err = snapshot.stable(client)
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalChanged)
	assert.Contains(t, err.Error(), "replaced during snapshot")
}

func TestJournalSnapshotDetectsInPlaceHeaderChange(t *testing.T) {
	client, journalDir, machineID := newJournalSnapshotTestClient(t)
	contents := buildJournalQueryFixture(t, []journalQueryFixtureEntry{
		{bootID: repeatedJournalID(0x22), realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "MESSAGE=ready"}},
	})
	path := writeJournalSnapshotFixture(t, journalDir, "system.journal", contents, machineID, repeatedJournalID(0x31), repeatedJournalID(0x44))

	snapshot, err := client.openJournalSnapshot()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.close()) })
	opened := snapshot.files[0]

	var changedRealtime [8]byte
	binary.LittleEndian.PutUint64(changedRealtime[:], opened.view.header.tailEntryRealtime+1)
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = writer.WriteAt(changedRealtime[:], 192)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, os.Chtimes(path, opened.info.ModTime(), opened.info.ModTime()))

	current, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, opened.info.Size(), current.Size())
	require.True(t, opened.info.ModTime().Equal(current.ModTime()))

	err = snapshot.stable(client)
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalChanged)
	assert.Contains(t, err.Error(), "header changed during snapshot")
}

func TestOpenJournalSnapshotRejectsTooManyFilesBeforeParsing(t *testing.T) {
	client, journalDir, _ := newJournalSnapshotTestClient(t)
	for index := 0; index <= maxJournalQueryFiles; index++ {
		path := filepath.Join(journalDir, fmt.Sprintf("%03d.journal", index))
		require.NoError(t, os.WriteFile(path, []byte("not a journal"), 0o600))
	}

	_, err := client.openJournalSnapshot()
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalLimit)
	assert.Contains(t, err.Error(), "maximum is 128")
}

func newJournalSnapshotTestClient(t *testing.T) (*Client, string, journalID) {
	t.Helper()
	root := t.TempDir()
	machineID := repeatedJournalID(0xa1)
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(machineID.String()+"\n"), 0o600))
	baseDir := filepath.Join(root, "journal")
	journalDir := filepath.Join(baseDir, machineID.String())
	require.NoError(t, os.MkdirAll(journalDir, 0o755))
	return NewClient(Target{JournalDirs: []string{baseDir}, MachineIDPath: machineIDPath}), journalDir, machineID
}

func writeJournalSnapshotFixture(t *testing.T, journalDir, name string, contents []byte, machineID, fileID, sequenceID journalID) string {
	t.Helper()
	contents = append([]byte(nil), contents...)
	copy(contents[24:40], fileID[:])
	copy(contents[40:56], machineID[:])
	copy(contents[72:88], sequenceID[:])
	path := filepath.Join(journalDir, name)
	require.NoError(t, os.WriteFile(path, contents, 0o600))
	return path
}
