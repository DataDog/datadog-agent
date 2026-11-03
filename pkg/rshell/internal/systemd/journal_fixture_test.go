// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const journalFixtureSize = 8 * 1024 * 1024

const journalFixtureDirectoryEnv = "RSHELL_JOURNAL_FIXTURE_DIR"

func TestRealJournalFixtures(t *testing.T) {
	longMessage := journalFixtureLongMessage()
	bootID := repeatedJournalID(0xbb)
	tests := []struct {
		name        string
		compact     bool
		keyedHash   bool
		compression uint8
	}{
		{name: "regular-uncompressed.journal.gz"},
		{
			name:        "compact-keyed-zstd.journal.gz",
			compact:     true,
			keyedHash:   true,
			compression: journalObjectCompressedZSTD,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contents := readJournalFixture(t, test.name)
			view, err := newJournalFileView(test.name, bytes.NewReader(contents), uint64(len(contents)))
			require.NoError(t, err)
			assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", view.header.machineID.String())
			assert.Equal(t, uint64(4), view.header.nEntries)
			assert.Equal(t, test.compact, view.header.compact())
			assert.Equal(t, test.keyedHash, view.header.keyedHash())

			messageData, found, err := view.findDataObject([]byte("MESSAGE=" + longMessage))
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, test.compression, messageData.flags)
			payload, truncated, err := view.readDataPayload(messageData, len(longMessage)+len("MESSAGE="))
			require.NoError(t, err)
			assert.False(t, truncated)
			assert.Equal(t, "MESSAGE="+longMessage, string(payload))

			unitIterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
				Units:       []string{"rshell-fixture.service"},
				CurrentBoot: true,
				MaxEntries:  10,
			}, &bootID)
			require.NoError(t, err)
			unitEntries := collectJournalQueryEntries(t, unitIterator)
			require.Len(t, unitEntries, 3)
			assert.Equal(t, []string{longMessage, "manager noticed service", "service started"}, journalMessages(unitEntries))
			assert.Equal(t, "systemd", unitEntries[1].selected.Identifier)
			assert.Equal(t, "1", unitEntries[1].selected.PID)

			kernelIterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
				Kernel:      true,
				CurrentBoot: true,
				MaxEntries:  10,
			}, &bootID)
			require.NoError(t, err)
			kernelEntries := collectJournalQueryEntries(t, kernelIterator)
			require.Len(t, kernelEntries, 1)
			assert.Equal(t, "synthetic kernel event", kernelEntries[0].selected.Message)
			assert.Equal(t, "kernel", kernelEntries[0].selected.Identifier)
		})
	}
}

func TestJournalFixtureMatchesJournalctl(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("journalctl differential check requires Linux")
	}
	journalctl, err := exec.LookPath("journalctl")
	if err != nil {
		t.Skip("journalctl is not installed")
	}

	path := filepath.Join(t.TempDir(), "fixture.journal")
	require.NoError(t, os.WriteFile(path, readJournalFixture(t, "regular-uncompressed.journal.gz"), 0o600))

	unitOutput := runFixtureJournalctl(t, journalctl, path, "--unit=rshell-fixture.service")
	assert.Equal(t, strings.Join([]string{
		"service started",
		"manager noticed service",
		journalFixtureLongMessage(),
		"",
	}, "\n"), unitOutput)

	kernelOutput := runFixtureJournalctl(t, journalctl, path, "--dmesg")
	assert.Equal(t, "synthetic kernel event\n", kernelOutput)
}

func TestReadJournalUsesPureGoFixtureBackend(t *testing.T) {
	root := t.TempDir()
	machineID := repeatedJournalID(0xaa)
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(machineID.String()+"\n"), 0o600))
	baseDir := filepath.Join(root, "journal")
	journalDir := filepath.Join(baseDir, machineID.String())
	require.NoError(t, os.MkdirAll(journalDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(journalDir, "system.journal"),
		readJournalFixture(t, "compact-keyed-zstd.journal.gz"),
		0o600,
	))

	client := NewClient(Target{JournalDirs: []string{baseDir}, MachineIDPath: machineIDPath})
	var entries []builtins.JournalEntry
	err := client.ReadJournal(context.Background(), builtins.JournalQuery{
		Units:       []string{"rshell-fixture.service"},
		CurrentBoot: true,
		MaxEntries:  2,
	}, func(entry builtins.JournalEntry) error {
		entries = append(entries, entry)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, []string{"manager noticed service", journalFixtureLongMessage()}, []string{entries[0].Message, entries[1].Message})
}

func TestReadJournalReturnsCanceledContextBeforeFilesystemAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewClient(Target{}).ReadJournal(ctx, builtins.JournalQuery{
		Units:      []string{"api.service"},
		MaxEntries: 1,
	}, func(builtins.JournalEntry) error { return nil })
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRealJournalFixtureRejectsCorruptedCompressedData(t *testing.T) {
	contents := readJournalFixture(t, "compact-keyed-zstd.journal.gz")
	view, err := newJournalFileView("corrupt-compressed.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	data, found, err := view.findDataObject([]byte("MESSAGE=" + journalFixtureLongMessage()))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, uint8(journalObjectCompressedZSTD), data.flags)
	contents[data.payloadOffset+data.payloadSize/2] ^= 0xff

	view, err = newJournalFileView("corrupt-compressed.journal", bytes.NewReader(contents), uint64(len(contents)))
	require.NoError(t, err)
	iterator, err := newJournalFileQueryIterator(view, builtins.JournalQuery{
		Units:      []string{"rshell-fixture.service"},
		MaxEntries: 10,
	}, nil)
	require.NoError(t, err)
	_, _, err = iterator.previous(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errJournalCorrupt)
}

func readJournalFixture(t *testing.T, name string) []byte {
	t.Helper()
	directory := os.Getenv(journalFixtureDirectoryEnv)
	if directory == "" {
		directory = filepath.Join("testdata", "journal")
	}
	file, err := os.Open(filepath.Join(directory, name))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	contents, err := io.ReadAll(io.LimitReader(reader, journalFixtureSize+1))
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Len(t, contents, journalFixtureSize)
	return contents
}

func journalFixtureLongMessage() string {
	return "compressed payload " + strings.TrimSpace(strings.Repeat(strings.Repeat("a", 64)+" ", 10))
}

func runFixtureJournalctl(t *testing.T, executable, journalPath string, selection ...string) string {
	t.Helper()
	arguments := []string{"--file=" + journalPath, "--no-pager", "--quiet", "--output=cat"}
	arguments = append(arguments, selection...)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	require.NoError(t, err, stderr.String())
	return string(output)
}
