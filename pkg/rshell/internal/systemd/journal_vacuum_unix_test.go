// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const testMachineID = "0123456789abcdef0123456789abcdef"

func archivedJournalName(index int) string {
	return fmt.Sprintf("system@abcdef0123456789abcdef0123456789-%016x-%016x.journal", index, index)
}

func corruptedJournalName(index int) string {
	return fmt.Sprintf("system@%016x-%016x.journal~", index, index)
}

func newVacuumTestClient(t *testing.T) (*Client, string) {
	t.Helper()
	root := t.TempDir()
	machineIDPath := filepath.Join(root, "machine-id")
	journalRoot := filepath.Join(root, "journal")
	machineDir := filepath.Join(journalRoot, testMachineID)
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(machineDir, 0o700))
	return NewClient(Target{JournalDirs: []string{journalRoot}, MachineIDPath: machineIDPath}), machineDir
}

func writeVacuumFile(t *testing.T, directory, name string, modTime time.Time) string {
	t.Helper()
	return writeVacuumFileOfSize(t, directory, name, modTime, 8192)
}

func writeVacuumFileOfSize(t *testing.T, directory, name string, modTime time.Time, size int) string {
	t.Helper()
	path := filepath.Join(directory, name)
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o600))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
	return path
}

func allocatedBytesOf(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	stat, err := journalStat(info)
	require.NoError(t, err)
	require.Greater(t, stat.allocated, uint64(0))
	return stat.allocated
}

func TestVacuumJournalDeletesOldestArchivesWithinRequest(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	client, directory := newVacuumTestClient(t)
	active := writeVacuumFile(t, directory, "system.journal", now.Add(-30*24*time.Hour))
	namespaceActive := writeVacuumFile(t, directory, "system@tenant.journal", now.Add(-30*24*time.Hour))
	malformed := writeVacuumFile(t, directory, "system@old.journal", now.Add(-30*24*time.Hour))
	oldest := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-10*24*time.Hour))
	old := writeVacuumFile(t, directory, archivedJournalName(2), now.Add(-5*24*time.Hour))
	recent := writeVacuumFile(t, directory, archivedJournalName(3), now.Add(-time.Hour))

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Files)
	assert.Greater(t, result.Bytes, uint64(0))
	assert.NoFileExists(t, oldest)
	assert.NoFileExists(t, old)
	assert.FileExists(t, recent)
	assert.FileExists(t, active)
	assert.FileExists(t, namespaceActive)
	assert.FileExists(t, malformed)
}

func TestVacuumJournalDryRunDoesNotDelete(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	archive := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-7*24*time.Hour))
	second := writeVacuumFile(t, directory, archivedJournalName(2), now.Add(-6*24*time.Hour))

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
		DryRun: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Files)
	assert.FileExists(t, archive)
	assert.FileExists(t, second)
}

func TestVacuumJournalDeletesCorruptionArchivesWithinRequest(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	old := writeVacuumFile(t, directory, corruptedJournalName(1), now.Add(-7*24*time.Hour))
	recent := writeVacuumFile(t, directory, corruptedJournalName(2), now.Add(-time.Hour))
	malformed := writeVacuumFile(t, directory, archivedJournalName(3)+"~", now.Add(-7*24*time.Hour))

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Files)
	assert.NoFileExists(t, old)
	assert.FileExists(t, recent)
	assert.FileExists(t, malformed)
}

func TestVacuumJournalHonorsAllocatedSizeTargetAndTimeFloor(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	oldest := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-10*24*time.Hour))
	second := writeVacuumFile(t, directory, archivedJournalName(2), now.Add(-9*24*time.Hour))
	recent := writeVacuumFile(t, directory, archivedJournalName(3), now.Add(-time.Hour))
	info, err := os.Lstat(oldest)
	require.NoError(t, err)
	stat, err := journalStat(info)
	require.NoError(t, err)
	require.Greater(t, stat.allocated, uint64(1))

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:      now,
		MaxBytes: stat.allocated - 1,
		Before:   now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Files)
	assert.NoFileExists(t, oldest)
	assert.NoFileExists(t, second)
	assert.FileExists(t, recent)
}

func TestVacuumJournalCombinedCleanupStopsAtSizeTarget(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	archive := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-10*24*time.Hour))
	info, err := os.Lstat(archive)
	require.NoError(t, err)
	stat, err := journalStat(info)
	require.NoError(t, err)

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:      now,
		MaxBytes: stat.allocated,
		Before:   now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Zero(t, result.Files)
	assert.FileExists(t, archive)
}

// The size target measures total allocated journal storage (active plus
// archived), matching --disk-usage and host journalctl --vacuum-size, rather
// than archived bytes alone.
func TestVacuumJournalSizeTargetCountsActiveAndArchivedAllocation(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	active := writeVacuumFileOfSize(t, directory, "system.journal", now.Add(-30*24*time.Hour), 5*8192)
	oldest := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-10*24*time.Hour))
	second := writeVacuumFile(t, directory, archivedJournalName(2), now.Add(-9*24*time.Hour))
	activeAllocated := allocatedBytesOf(t, active)
	oldestAllocated := allocatedBytesOf(t, oldest)
	secondAllocated := allocatedBytesOf(t, second)
	// Archived bytes alone (oldest+second) are already below this target, so the
	// pre-fix archived-only accounting would have deleted nothing at all.
	maxBytes := activeAllocated + secondAllocated
	require.Less(t, oldestAllocated+secondAllocated, maxBytes)

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:      now,
		MaxBytes: maxBytes,
		Before:   now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Files)
	assert.Equal(t, oldestAllocated, result.Bytes)
	assert.Equal(t, maxBytes, result.RemainingBytes)
	assert.NoFileExists(t, oldest)
	assert.FileExists(t, second)
	assert.FileExists(t, active)
}

func TestVacuumJournalNeverDeletesActiveJournalsForSizeTarget(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	active := writeVacuumFileOfSize(t, directory, "system.journal", now.Add(-30*24*time.Hour), 5*8192)
	namespaceActive := writeVacuumFile(t, directory, "system@tenant.journal", now.Add(-30*24*time.Hour))
	archive := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-10*24*time.Hour))
	remaining := allocatedBytesOf(t, active) + allocatedBytesOf(t, namespaceActive)

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:      now,
		MaxBytes: 1,
		Before:   now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Files)
	assert.FileExists(t, active)
	assert.FileExists(t, namespaceActive)
	assert.NoFileExists(t, archive)
	// The target is unreachable without deleting active journals; the result
	// reports the remaining allocation instead of implying success.
	assert.Equal(t, remaining, result.RemainingBytes)
	assert.Greater(t, result.RemainingBytes, uint64(1))
}

func TestVacuumJournalTimeCutoffOutranksSizeTarget(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	active := writeVacuumFileOfSize(t, directory, "system.journal", now.Add(-30*24*time.Hour), 5*8192)
	recent := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-time.Hour))

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:      now,
		MaxBytes: 1,
		Before:   now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Zero(t, result.Files)
	assert.Zero(t, result.Bytes)
	assert.Equal(t, allocatedBytesOf(t, active)+allocatedBytesOf(t, recent), result.RemainingBytes)
	assert.FileExists(t, active)
	assert.FileExists(t, recent)
}

func TestVacuumJournalSkipsHardlinksAndSymlinks(t *testing.T) {
	now := time.Now()
	client, directory := newVacuumTestClient(t)
	first := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-10*24*time.Hour))
	second := filepath.Join(directory, archivedJournalName(2))
	require.NoError(t, os.Link(first, second))
	symlink := filepath.Join(directory, archivedJournalName(3))
	require.NoError(t, os.Symlink(first, symlink))

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Zero(t, result.Files)
	assert.FileExists(t, first)
	assert.FileExists(t, second)
	_, err = os.Lstat(symlink)
	require.NoError(t, err)
}

func TestVacuumJournalRejectsSymlinkedMachineDirectory(t *testing.T) {
	now := time.Now()
	root := t.TempDir()
	journalRoot := filepath.Join(root, "journal")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(journalRoot, 0o700))
	require.NoError(t, os.MkdirAll(outside, 0o700))
	archive := writeVacuumFile(t, outside, archivedJournalName(1), now.Add(-10*24*time.Hour))
	require.NoError(t, os.Symlink(outside, filepath.Join(journalRoot, testMachineID)))
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))
	client := NewClient(Target{JournalDirs: []string{journalRoot}, MachineIDPath: machineIDPath})

	_, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a real directory")
	assert.FileExists(t, archive)
}

func TestVacuumJournalRequiresRequestBounds(t *testing.T) {
	now := time.Now()
	client, _ := newVacuumTestClient(t)
	_, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{Before: now.Add(-time.Hour)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference time")

	_, err = client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{Now: now})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a size or time limit")

	_, err = client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{Now: now, MaxBytes: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size vacuum requires a time cutoff")

	_, err = client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{Now: now, Before: now.Add(time.Hour)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be in the future")
}

// cancelAfterContext cancels a real context once VacuumJournal has polled
// ctx.Err() the configured number of times. The loop in VacuumJournal checks
// cancellation once per candidate, so remaining=1 cancels the run after the
// first archive has already been deleted. This keeps the "cancelled mid-loop"
// path deterministic without adding a hook to the production code: the error
// still comes from a genuine cancelled context.
type cancelAfterContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func newCancelAfterContext(t *testing.T, remaining int) *cancelAfterContext {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &cancelAfterContext{Context: ctx, cancel: cancel, remaining: remaining}
}

func (c *cancelAfterContext) Err() error {
	if c.remaining > 0 {
		c.remaining--
	} else {
		c.cancel()
	}
	return c.Context.Err()
}

// newVacuumCandidate builds the candidate record VacuumJournal would have
// collected for name, so revalidateVacuumCandidate can be exercised against a
// file mutated after collection.
func newVacuumCandidate(t *testing.T, directory, name string) vacuumCandidate {
	t.Helper()
	root, err := os.OpenRoot(directory)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	info, err := root.Lstat(name)
	require.NoError(t, err)
	stat, err := journalStat(info)
	require.NoError(t, err)
	return vacuumCandidate{
		directory: &vacuumDirectory{path: directory, root: root},
		name:      name,
		modTime:   info.ModTime(),
		size:      info.Size(),
		stat:      stat,
	}
}

func TestVacuumPartialErrorReportsCompletedDeletions(t *testing.T) {
	cause := errors.New("boom")

	t.Run("no deletions returns the cause unchanged", func(t *testing.T) {
		err := vacuumPartialError(builtins.JournalVacuumResult{}, cause)
		require.Equal(t, cause, err)
		require.ErrorIs(t, err, cause)
		assert.NotContains(t, err.Error(), "journal vacuum stopped")
	})

	t.Run("completed deletions are named and the cause is preserved", func(t *testing.T) {
		err := vacuumPartialError(builtins.JournalVacuumResult{Files: 3, Bytes: 24576}, cause)
		require.Error(t, err)
		require.ErrorIs(t, err, cause)
		assert.Equal(t, cause, errors.Unwrap(err))
		assert.Equal(t, "journal vacuum stopped after deleting 3 files (24576 bytes): boom", err.Error())
	})
}

func TestVacuumJournalCancellationMidLoopReportsPartialProgress(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	client, directory := newVacuumTestClient(t)
	oldest := writeVacuumFile(t, directory, archivedJournalName(1), now.Add(-10*24*time.Hour))
	second := writeVacuumFile(t, directory, archivedJournalName(2), now.Add(-9*24*time.Hour))
	third := writeVacuumFile(t, directory, archivedJournalName(3), now.Add(-8*24*time.Hour))

	ctx := newCancelAfterContext(t, 1)
	result, err := client.VacuumJournal(ctx, builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "journal vacuum stopped after deleting 1 files")
	assert.Contains(t, err.Error(), fmt.Sprintf("(%d bytes)", result.Bytes))

	assert.Equal(t, 1, result.Files)
	assert.Greater(t, result.Bytes, uint64(0))
	assert.NoFileExists(t, oldest)
	assert.FileExists(t, second)
	assert.FileExists(t, third)
}

func TestVacuumJournalRevalidationFailureMidLoopReportsPartialProgress(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	machineIDPath := filepath.Join(root, "machine-id")
	journalRoot := filepath.Join(root, "journal")
	machineDir := filepath.Join(journalRoot, testMachineID)
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(machineDir, 0o700))
	oldest := writeVacuumFile(t, machineDir, archivedJournalName(1), now.Add(-10*24*time.Hour))
	second := writeVacuumFile(t, machineDir, archivedJournalName(2), now.Add(-9*24*time.Hour))

	// Listing the same journal directory twice makes every archive appear as
	// two candidates backed by one inode. Deleting the first copy leaves the
	// duplicate stale, which is exactly the state a concurrent external
	// deletion produces, and it drives revalidateVacuumCandidate's failure
	// path deterministically: the candidate list is built inside the call, so
	// a test cannot mutate it from the outside between iterations.
	client := NewClient(Target{JournalDirs: []string{journalRoot, journalRoot}, MachineIDPath: machineIDPath})

	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal vacuum stopped after deleting 1 files")
	assert.Contains(t, err.Error(), "revalidate archived journal")
	assert.Equal(t, 1, result.Files)
	assert.Greater(t, result.Bytes, uint64(0))
	assert.NoFileExists(t, oldest)
	assert.FileExists(t, second)
}

// The remove-failure branch (root.Remove returning an error mid-loop) is not
// covered on purpose: forcing it needs a parent directory that denies unlink,
// which a root-owned CI container ignores, so any such test would be
// platform-fragile. The wrapping it applies is the same vacuumPartialError
// wrapping asserted by TestVacuumPartialErrorReportsCompletedDeletions.

func TestRevalidateVacuumCandidateAcceptsUnchangedArchive(t *testing.T) {
	directory := t.TempDir()
	name := archivedJournalName(1)
	writeVacuumFile(t, directory, name, time.Now().Add(-10*24*time.Hour))
	require.NoError(t, revalidateVacuumCandidate(newVacuumCandidate(t, directory, name)))
}

func TestRevalidateVacuumCandidateRejectsChangedArchive(t *testing.T) {
	modTime := time.Date(2026, time.July, 4, 12, 0, 0, 0, time.UTC)
	name := archivedJournalName(1)
	tests := []struct {
		name    string
		mutate  func(t *testing.T, directory, path string)
		message string
	}{
		{
			name: "deleted before removal",
			mutate: func(t *testing.T, _, path string) {
				require.NoError(t, os.Remove(path))
			},
			message: "revalidate archived journal",
		},
		{
			name: "size changed",
			mutate: func(t *testing.T, _, path string) {
				require.NoError(t, os.WriteFile(path, make([]byte, 4096), 0o600))
				require.NoError(t, os.Chtimes(path, modTime, modTime))
			},
			message: "archived journal changed before deletion",
		},
		{
			name: "modification time changed",
			mutate: func(t *testing.T, _, path string) {
				newer := modTime.Add(time.Hour)
				require.NoError(t, os.Chtimes(path, newer, newer))
			},
			message: "archived journal changed before deletion",
		},
		{
			name: "replaced by a symlink",
			mutate: func(t *testing.T, directory, path string) {
				target := filepath.Join(directory, "target")
				require.NoError(t, os.WriteFile(target, make([]byte, 8192), 0o600))
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Symlink(target, path))
			},
			message: "archived journal changed before deletion",
		},
		{
			name: "replaced by a directory",
			mutate: func(t *testing.T, _, path string) {
				require.NoError(t, os.Remove(path))
				require.NoError(t, os.Mkdir(path, 0o700))
			},
			message: "archived journal changed before deletion",
		},
		{
			// A filesystem can immediately recycle a just-unlinked inode for
			// the replacement file, so os.Remove followed by os.WriteFile
			// cannot be relied on to produce a different dev/ino: on such a
			// filesystem the replacement would keep the same identity, size,
			// and mtime as candidate and revalidateVacuumCandidate would
			// wrongly accept it. Instead this replaces the file with a
			// sparse file of the same apparent size: candidate's original
			// content was fully written (real disk blocks allocated), so a
			// truncate-only file of equal size reports a different block
			// count, deterministically tripping the identity check
			// regardless of inode reuse.
			name: "replaced with a sparse file of the same size",
			mutate: func(t *testing.T, _, path string) {
				require.NoError(t, os.Remove(path))
				f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
				require.NoError(t, err)
				require.NoError(t, f.Truncate(8192))
				require.NoError(t, f.Close())
				require.NoError(t, os.Chtimes(path, modTime, modTime))
			},
			message: "archived journal identity changed before deletion",
		},
		{
			name: "hardlinked after collection",
			mutate: func(t *testing.T, directory, path string) {
				require.NoError(t, os.Link(path, filepath.Join(directory, archivedJournalName(2))))
				require.NoError(t, os.Chtimes(path, modTime, modTime))
			},
			message: "archived journal identity changed before deletion",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := writeVacuumFile(t, directory, name, modTime)
			candidate := newVacuumCandidate(t, directory, name)
			test.mutate(t, directory, path)

			err := revalidateVacuumCandidate(candidate)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestVacuumJournalSpansEveryConfiguredJournalDirectory(t *testing.T) {
	now := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	modTime := now.Add(-10 * 24 * time.Hour)
	root := t.TempDir()
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))

	persistent := filepath.Join(root, "log-journal", testMachineID)
	volatile := filepath.Join(root, "run-journal", testMachineID)
	require.NoError(t, os.MkdirAll(persistent, 0o700))
	require.NoError(t, os.MkdirAll(volatile, 0o700))
	// Same modification time in both directories, so the ordering tie is
	// broken by the journal directory path.
	first := writeVacuumFile(t, persistent, archivedJournalName(1), modTime)
	second := writeVacuumFile(t, volatile, archivedJournalName(1), modTime)
	// A directory carrying an archived journal name is never a candidate.
	decoy := filepath.Join(volatile, archivedJournalName(2))
	require.NoError(t, os.Mkdir(decoy, 0o700))

	client := NewClient(Target{
		JournalDirs:   []string{filepath.Dir(persistent), filepath.Dir(volatile)},
		MachineIDPath: machineIDPath,
	})
	result, err := client.VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Files)
	assert.NoFileExists(t, first)
	assert.NoFileExists(t, second)
	assert.DirExists(t, decoy)
}

func TestOpenVacuumDirectoriesRequiresTargetConfiguration(t *testing.T) {
	root := t.TempDir()
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))

	_, err := NewClient(Target{JournalDirs: []string{root}}).openVacuumDirectories()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "machine ID path is not configured")

	_, err = NewClient(Target{MachineIDPath: machineIDPath}).openVacuumDirectories()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "journal directories are not configured")

	_, err = NewClient(Target{
		JournalDirs:   []string{root},
		MachineIDPath: filepath.Join(root, "missing-machine-id"),
	}).openVacuumDirectories()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open systemd machine ID")
}

func TestOpenVacuumDirectoriesSkipsMissingRootsAndMachineDirectories(t *testing.T) {
	root := t.TempDir()
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))
	withoutMachineDir := filepath.Join(root, "journal")
	require.NoError(t, os.MkdirAll(withoutMachineDir, 0o700))

	client := NewClient(Target{
		JournalDirs:   []string{filepath.Join(root, "absent"), withoutMachineDir},
		MachineIDPath: machineIDPath,
	})
	directories, err := client.openVacuumDirectories()
	require.NoError(t, err)
	assert.Empty(t, directories)
	closeVacuumDirectories(directories)
}

func TestOpenVacuumDirectoriesRejectsUnopenableJournalRoot(t *testing.T) {
	root := t.TempDir()
	machineIDPath := filepath.Join(root, "machine-id")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))
	usable := filepath.Join(root, "journal")
	require.NoError(t, os.MkdirAll(filepath.Join(usable, testMachineID), 0o700))
	notADirectory := filepath.Join(root, "journal-file")
	require.NoError(t, os.WriteFile(notADirectory, []byte("not a directory"), 0o600))

	// The usable root is opened first so the failure on the second root also
	// exercises the cleanup of the already-pinned directories.
	client := NewClient(Target{
		JournalDirs:   []string{usable, notADirectory},
		MachineIDPath: machineIDPath,
	})
	directories, err := client.openVacuumDirectories()
	require.Error(t, err)
	assert.Nil(t, directories)
	assert.Contains(t, err.Error(), "open journal root")
}

func TestOpenVacuumDirectoriesRejectsNonDirectoryMachineEntry(t *testing.T) {
	root := t.TempDir()
	machineIDPath := filepath.Join(root, "machine-id")
	journalRoot := filepath.Join(root, "journal")
	require.NoError(t, os.WriteFile(machineIDPath, []byte(testMachineID+"\n"), 0o600))
	require.NoError(t, os.MkdirAll(journalRoot, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(journalRoot, testMachineID), []byte("regular file"), 0o600))

	client := NewClient(Target{JournalDirs: []string{journalRoot}, MachineIDPath: machineIDPath})
	directories, err := client.openVacuumDirectories()
	require.Error(t, err)
	assert.Nil(t, directories)
	assert.Contains(t, err.Error(), "journal machine directory is not a real directory")
}

// The machine-directory identity check at the end of openVacuumDirectories
// (dev/ino compared across Lstat and OpenRoot) has no deterministic test: it
// only trips when the directory is swapped inside that window, and losing the
// race reliably would need a production-code hook. The equivalent per-file
// identity check is covered by TestRevalidateVacuumCandidateRejectsChangedArchive.

func TestIsArchivedJournalName(t *testing.T) {
	for _, name := range []string{
		archivedJournalName(1),
		"system@tenant@abcdef0123456789abcdef0123456789-0000000000000001-0000000000000001.journal",
		"user-1000@abcdef0123456789abcdef0123456789-0000000000000001-0000000000000001.journal",
		corruptedJournalName(1),
		"system@tenant@0000000000000001-0000000000000001.journal~",
		"user-1000@0000000000000001-0000000000000001.journal~",
	} {
		assert.True(t, isArchivedJournalName(name), name)
	}
	for _, name := range []string{
		"system.journal",
		"user-1000.journal",
		"system@tenant.journal",
		"system@old.journal",
		"system@abcdef0123456789abcdef0123456789-1-1.journal",
		"system@abcdef0123456789abcdef0123456789-0000000000000001-0000000000000001.journal~",
		"system@0000000000000001-1.journal~",
		"system@0000000000000001-0000000000000001.journal~~",
		"../" + archivedJournalName(1),
	} {
		assert.False(t, isArchivedJournalName(name), name)
	}
}
