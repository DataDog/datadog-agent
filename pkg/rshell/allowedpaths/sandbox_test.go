// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDirEntry is a minimal fs.DirEntry for testing CollectDirEntries.
type fakeDirEntry struct {
	name string
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return false }
func (f fakeDirEntry) Type() fs.FileMode          { return 0 }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return fakeFileInfo{name: f.name}, nil }

type fakeFileInfo struct{ name string }

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0644 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

// TestSandboxDefaultReadOnly verifies that a freshly created sandbox blocks
// write opens without an explicit SetWritable call — defense-in-depth so
// that even if the interpreter accidentally passes write flags in read-only
// mode, the sandbox rejects them.
func TestSandboxDefaultReadOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("data"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()

	writeFlags := []int{
		os.O_WRONLY,
		os.O_WRONLY | os.O_CREATE | os.O_TRUNC,
		os.O_WRONLY | os.O_APPEND,
		os.O_WRONLY | os.O_CREATE,
	}
	for _, flag := range writeFlags {
		f, err := sb.Open("existing.txt", dir, flag, 0644)
		assert.Nil(t, f, "read-only sandbox must reject flag %d", flag)
		assert.ErrorIs(t, err, os.ErrPermission, "read-only sandbox must return ErrPermission for flag %d", flag)
	}
}

func TestSandboxWriteAllowedPath(t *testing.T) {
	dir := t.TempDir()

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	// O_CREATE|O_WRONLY for a new file inside the allowlist should succeed
	// and the file's contents should reflect the write.
	f, err := sb.Open("created.txt", dir, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	require.NoError(t, err)
	n, err := f.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(filepath.Join(dir, "created.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

func TestSandboxWriteOutsideAllowedPath(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	sb, _, err := New([]string{allowed + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	// Absolute path outside the allowlist must be rejected for writes.
	target := filepath.Join(outside, "should-not-exist.txt")
	f, err := sb.Open(target, allowed, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	assert.Nil(t, f)
	assert.ErrorIs(t, err, os.ErrPermission)

	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr), "file outside allowlist must not be created")
}

func TestSandboxAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	require.NoError(t, os.WriteFile(path, []byte("first\n"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	f, err := sb.Open("log.txt", dir, os.O_WRONLY|os.O_APPEND, 0)
	require.NoError(t, err)
	_, err = f.Write([]byte("second\n"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "first\nsecond\n", string(got))
}

func TestSandboxTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original-content"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	f, err := sb.Open("data.txt", dir, os.O_WRONLY|os.O_TRUNC, 0)
	require.NoError(t, err)
	_, err = f.Write([]byte("short"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "short", string(got), "O_TRUNC must replace, not append to, the original content")
}

func TestSandboxWriteRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original content that is long"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	mutated, err := sb.WriteRegularFile(context.Background(), "data.txt", dir, []byte("short"), nil)
	require.NoError(t, err)
	assert.True(t, mutated, "a successful write must report having mutated the file")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "short", string(got),
		"the new, shorter content must fully replace the original, with no stale tail")
}

func TestSandboxWriteRegularFileLongerContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("short"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), "data.txt", dir, []byte("much longer replacement content"), nil)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "much longer replacement content", string(got))
}

func TestSandboxWriteRegularFileReadOnlyRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	// SetWritable is intentionally not called: the sandbox defaults to
	// read-only, and WriteRegularFile must refuse a write in that mode
	// exactly like Open/Truncate do.

	_, err = sb.WriteRegularFile(context.Background(), "data.txt", dir, []byte("new"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrPermission))

	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(got), "read-only mode must leave the file untouched")
}

func TestSandboxWriteRegularFileMissingFileNotCreated(t *testing.T) {
	dir := t.TempDir()

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), "missing.txt", dir, []byte("new"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	_, statErr := os.Stat(filepath.Join(dir, "missing.txt"))
	assert.True(t, os.IsNotExist(statErr), "WriteRegularFile must not create a missing file")
}

func TestSandboxWriteRegularFileOutsideAllowedPathsRejected(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	path := filepath.Join(other, "secret.txt")
	require.NoError(t, os.WriteFile(path, []byte("secret"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), path, dir, []byte("pwned"), nil)
	require.Error(t, err)

	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "secret", string(got))
}

func TestSandboxWriteRegularFileRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0755))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), "subdir", dir, []byte("new"), nil)
	require.Error(t, err)
}

func TestSandboxWriteRegularFileNilSandbox(t *testing.T) {
	var sb *Sandbox
	_, err := sb.WriteRegularFile(context.Background(), "data.txt", "/tmp", []byte("new"), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrPermission))
}

// TestSandboxWriteRegularFileRefusesAlreadyCancelledContext verifies that a
// context cancelled before the call is checked up front, before any file is
// opened or mutated.
func TestSandboxWriteRegularFileRefusesAlreadyCancelledContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mutated, err := sb.WriteRegularFile(ctx, "data.txt", dir, []byte("new"), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, mutated, "an already-cancelled context must not report having mutated the file")

	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(got), "an already-cancelled context must leave the file untouched")
}

// TestSandboxWriteRegularFileStopsMidWriteOnCancellation is a regression
// test for the P2 finding: a write in progress must be interruptible by
// context cancellation partway through, not just refused up front, since a
// large write (sed -i's rewrite can be up to 256 MiB) could otherwise run to
// completion past the caller's deadline. Builds data spanning multiple
// writeRegularFileChunkBytes-sized chunks and cancels the context after the
// first chunk would have been written, then verifies the write stopped
// (returned context.Canceled) rather than writing the whole payload.
func TestSandboxWriteRegularFileStopsMidWriteOnCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0644))

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)

	// cancelAfterNCalls cancels itself on the Nth call to Err(), simulating
	// cancellation arriving exactly between two chunk writes rather than
	// before the call starts at all. writeChunkedCancellable calls Err()
	// once per chunk before writing that chunk (call 1 for the first chunk,
	// call 2 for the second, ...). cancelAfter:2 lets the first chunk's
	// check pass, so exactly one full chunk is written before the second
	// chunk's check observes cancellation — genuinely exercising the
	// mid-write stop, not just the already-cancelled-before-starting case
	// covered by the preceding test.
	//
	// Exercises writeAndTruncateSync directly, not the full
	// WriteRegularFile → writeAndTruncateBounded race: that outer race
	// deliberately no longer reads ctx.Err() itself (it selects on
	// ctx.Done() instead, see writeAndTruncateBounded's doc), so driving
	// cancellation through an exact Err() call count only pins the
	// synchronous implementation's own internal behavior deterministically
	// — the bounded wrapper's race/abandon behavior is covered separately
	// by TestWriteAndTruncateBoundedAbandonsOnPermanentStall below.
	ctx, cancel := context.WithCancel(context.Background())
	cc := &cancelAfterNCalls{Context: ctx, cancel: cancel, cancelAfter: 2}

	data := make([]byte, writeRegularFileChunkBytes*3)
	for i := range data {
		data[i] = 'a'
	}

	mutated, err := writeAndTruncateSync(cc, f, data)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, mutated, "a write that already landed at least one full chunk must report having mutated the file")

	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Less(t, len(got), len(data), "the write must have stopped before writing the full payload")
	assert.GreaterOrEqual(t, len(got), writeRegularFileChunkBytes, "at least one full chunk must have been written before cancellation was observed")
}

// partialThenFailWriter simulates an io.Writer that writes part of a single
// Write call's input before failing (e.g. ENOSPC or a quota limit hit mid-
// write) — a real possibility per io.Writer's documented contract (Write
// may return n > 0 alongside a non-nil error), needed to pin the P1 fix
// below.
type partialThenFailWriter struct {
	writeN int
	err    error
}

func (w *partialThenFailWriter) Write(p []byte) (int, error) {
	n := w.writeN
	if n > len(p) {
		n = len(p)
	}
	return n, w.err
}

// TestWriteChunkedCancellableTreatsPartialWriteAsMutation is a regression
// test for a P1 finding: writeChunkedCancellable was discarding the byte
// count from a failed Write call entirely, so a Write that returned n > 0
// alongside a non-nil error (a partial write, not a cancellation) was
// reported as wroteAny=false — telling writeBack the file was never
// touched, when in fact it was partially overwritten, so writeBack would
// skip the restore-on-failure path entirely and leave the file with
// unrecoverable partial content.
func TestWriteChunkedCancellableTreatsPartialWriteAsMutation(t *testing.T) {
	writeErr := errors.New("no space left on device")
	w := &partialThenFailWriter{writeN: 3, err: writeErr}

	wroteAny, err := writeChunkedCancellable(context.Background(), w, []byte("some data to write"))
	require.Error(t, err)
	assert.ErrorIs(t, err, writeErr)
	assert.True(t, wroteAny, "a partial write (n > 0 alongside an error) must still be reported as having mutated the target")
}

// TestWriteChunkedCancellableNoWriteIsNotAMutation is the converse: a
// Write call that fails without writing anything at all (n == 0) must not
// be reported as a mutation.
func TestWriteChunkedCancellableNoWriteIsNotAMutation(t *testing.T) {
	writeErr := errors.New("no space left on device")
	w := &partialThenFailWriter{writeN: 0, err: writeErr}

	wroteAny, err := writeChunkedCancellable(context.Background(), w, []byte("some data to write"))
	require.Error(t, err)
	assert.ErrorIs(t, err, writeErr)
	assert.False(t, wroteAny, "a Write call that wrote zero bytes before failing must not be reported as having mutated the target")
}

// TestSandboxWriteRegularFileRechecksCancellationBeforeTruncatingEmptyWrite
// is a regression test for a P2 finding: for empty data (e.g. sed -i 'd'
// file, whose rewritten output is nothing), writeChunkedCancellable's
// per-chunk loop body never runs at all — there are no chunks — so it never
// observes cancellation that arrived during the (potentially slow) path
// resolution, open, and fstat steps that precede it. Without an explicit
// recheck immediately before the truncate (the operation that actually
// destroys the original content for an empty rewrite), a cancellation
// landing in that window would still let the truncate proceed and report
// success, leaving writeBack with no failure to trigger its restore path.
//
// Exercises writeAndTruncateSync directly (see
// TestSandboxWriteRegularFileStopsMidWriteOnCancellation's comment for why
// the exact-call-count timing this test relies on can no longer be driven
// deterministically through the full WriteRegularFile →
// writeAndTruncateBounded path). writeAndTruncateSync calls ctx.Err()
// exactly once for empty data (immediately before the truncate, since the
// chunk loop contributes no calls when data is empty). cancelAfter:1
// cancels exactly at that check, simulating cancellation arriving during
// the (potentially slow) path resolution/open/fstat/acquisition steps a
// real caller would have performed before this point, rather than before
// the call even starts.
func TestSandboxWriteRegularFileRechecksCancellationBeforeTruncatingEmptyWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original content"), 0644))

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cc := &cancelAfterNCalls{Context: ctx, cancel: cancel, cancelAfter: 1}

	mutated, err := writeAndTruncateSync(cc, f, []byte(""))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, mutated, "cancellation caught before the truncate ever ran must not report having mutated the file")

	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original content", string(got),
		"cancellation observed just before the truncate must stop it from destroying the original content")
}

// TestRaceAcquisitionAgainstContextHonorsCancellationBeforeAcquisitionCompletes
// is a regression test for a P2 finding: WriteRegularFile's target
// resolution and open (resolveWriteTarget/openWriteFile, both syscalls
// that can block on a stalled FUSE/network-backed AllowedPaths root) had
// no cancellation hook at all before this fix — watchContextCloseOnDone
// only ever bounds operations on an already-open descriptor, installed
// only after acquisition already completed. Uses a fake acquire function
// that blocks until unblocked, verifying the race returns promptly on
// ctx cancellation rather than waiting for the slow acquisition to
// finish.
func TestRaceAcquisitionAgainstContextHonorsCancellationBeforeAcquisitionCompletes(t *testing.T) {
	unblock := make(chan struct{})
	t.Cleanup(func() { close(unblock) }) // let the abandoned goroutine finish so it doesn't leak past the test

	acquire := func() (*os.File, error) {
		<-unblock // simulates a resolve/open syscall stalled on a hung filesystem
		return nil, errors.New("unused")
	}

	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := raceAcquisitionAgainstContext(ctx, acquire)
	elapsed := time.Since(start)

	require.Error(t, err, "a stalled acquisition must be bounded by ctx cancellation rather than blocking the race forever")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, elapsed, 2*time.Second, "must return promptly once ctx is cancelled, not wait for the stalled acquisition to complete")
}

// TestRaceAcquisitionAgainstContextReturnsResultWhenFasterThanCancellation
// verifies the converse: a normal, fast acquisition is unaffected by the
// race and returns its real result.
func TestRaceAcquisitionAgainstContextReturnsResultWhenFasterThanCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	f, err := os.Create(path)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })

	acquire := func() (*os.File, error) { return f, nil }

	got, err := raceAcquisitionAgainstContext(context.Background(), acquire)
	require.NoError(t, err)
	assert.Same(t, f, got)
}

// TestRaceAcquisitionAgainstContextCapsOutstandingAbandonedAcquisitions fills
// the real slot pool with abandoned acquisitions, checks that another call
// fails fast, then releases the blocked acquisitions during cleanup.
func TestRaceAcquisitionAgainstContextCapsOutstandingAbandonedAcquisitions(t *testing.T) {
	// Earlier tests may still have abandoned acquisitions finishing cleanup.
	// Never replace the pool while their goroutines can access it.
	require.Eventually(t, func() bool { return len(writeAcquisitionSlots) == 0 }, 2*time.Second, time.Millisecond)
	unblock := make(chan struct{})
	t.Cleanup(func() {
		close(unblock)
		assert.Eventually(t, func() bool { return len(writeAcquisitionSlots) == 0 }, 2*time.Second, time.Millisecond)
	})

	blockedAcquire := func() (*os.File, error) {
		<-unblock
		return nil, errors.New("acquisition released by test cleanup")
	}

	alreadyCancelled, cancel := context.WithCancel(context.Background())
	cancel()

	// Fill every slot with an acquisition that stays blocked until cleanup.
	for i := 0; i < cap(writeAcquisitionSlots); i++ {
		_, err := raceAcquisitionAgainstContext(alreadyCancelled, blockedAcquire)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	}

	// No acquisition can complete yet, so the next call must fail fast.
	fastAcquire := func() (*os.File, error) { return nil, nil }
	_, err := raceAcquisitionAgainstContext(context.Background(), fastAcquire)
	require.Error(t, err, "a call made while every slot is held by a permanently-stuck abandoned acquisition must fail fast, not block or silently exceed the cap")
	assert.Contains(t, err.Error(), "too many in-flight write acquisitions")
}

// TestSandboxWriteRegularFileAcquisitionHonorsCancelledContext is the
// integration-level counterpart, exercising WriteRegularFile's own,
// already-cancelled-before-acquisition path (a stalled acquisition itself
// cannot be deterministically reproduced against a real, healthy
// filesystem, unlike the direct raceAcquisitionAgainstContext tests above,
// but this at minimum pins that WriteRegularFile's acquisition step goes
// through the ctx-aware race path rather than a plain, unbounded call).
func TestSandboxWriteRegularFileAcquisitionHonorsCancelledContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	// cancelAfter:1 means the very first ctx.Err() check — which
	// WriteRegularFile's own upfront check performs — triggers cancellation.
	// That upfront check already existed before this round's fix, so this
	// alone does not distinguish old from new behavior; it exists here
	// mainly to confirm WriteRegularFile still behaves correctly (untouched
	// file, cancellation error) with the new acquisition path in place.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = sb.WriteRegularFile(ctx, "data.txt", dir, []byte("new"), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(got))
}

// TestWriteAndTruncateBoundedAbandonsOnPermanentStall is a regression test
// for a P2 finding: closing f from another goroutine
// (watchContextCloseOnDone, this function's now-removed predecessor) is
// not a sufficient bound on a regular file, since Write/Truncate on it go
// through ordinary blocking syscalls that Go's runtime network poller does
// not intercept the way it does for pollable descriptors (pipes, sockets).
// writeAndTruncateBounded instead races the whole write+truncate+close
// sequence in a separate goroutine against ctx.Done(), so *this* call can
// still return once ctx is done even when the underlying sync call itself
// never will.
//
// Simulates a permanently stuck regular-file write via a fake
// *os.File-shaped write target is impractical (writeAndTruncateSync's
// signature is *os.File-specific), so this test instead pins the
// observable contract directly: given an already-cancelled ctx,
// writeAndTruncateBounded must return promptly with ctx.Err() and
// mutated=true (conservative, since the abandoned goroutine may still be
// mutating f), never blocking on the synchronous call's own completion.
func TestWriteAndTruncateBoundedAbandonsOnPermanentStall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0644))
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	mutated, werr := writeAndTruncateBounded(ctx, f, []byte("new content"))
	elapsed := time.Since(start)

	require.Error(t, werr)
	assert.ErrorIs(t, werr, context.Canceled)
	assert.True(t, mutated, "an abandoned write must conservatively report mutated=true, since the write may still be landing bytes in the background")
	assert.Less(t, elapsed, 2*time.Second, "writeAndTruncateBounded must return promptly once ctx is done, not wait for the synchronous write to complete")

	// The abandoned goroutine still runs to completion against the real f
	// in the background (writeAndTruncateSync's own ctx.Err() check will
	// observe the already-cancelled ctx and stop before writing anything,
	// then close f) — give it a moment to finish so it doesn't race with
	// this test's own process exit.
	time.Sleep(50 * time.Millisecond)
}

// A completed write must make its slot reusable before its result is observed.
func TestWriteAndTruncateBoundedReleasesSlotBeforeReturning(t *testing.T) {
	require.Eventually(t, func() bool { return len(writeAcquisitionSlots) == 0 }, 2*time.Second, time.Millisecond)
	reserved := cap(writeAcquisitionSlots) - 1
	for range reserved {
		writeAcquisitionSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for range reserved {
			<-writeAcquisitionSlots
		}
		assert.Eventually(t, func() bool { return len(writeAcquisitionSlots) == 0 }, 2*time.Second, time.Millisecond)
	})

	path := filepath.Join(t.TempDir(), "data.txt")
	for range 1000 {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		require.NoError(t, err)
		mutated, err := writeAndTruncateBounded(context.Background(), f, []byte("x"))
		occupied := len(writeAcquisitionSlots)
		require.NoError(t, err)
		require.True(t, mutated)
		require.Equal(t, reserved, occupied, "completed write still holds its slot")
	}
}

// TestWaitForWriteOutcomeResolvesOnceAbandonedWriterFinishes is a
// regression test for a P1 finding: WriteRegularFile's caller (sed -i's
// writeBack, via interp's writeRegularFile wrapper) was permanently
// treating an abandoned write as unrecoverable the instant ctx became
// done, even though the abandoned goroutine keeps running and may in fact
// finish moments later with a perfectly ordinary, safely restorable
// result. WaitForWriteOutcome must let a caller learn that real result
// when it arrives within the given timeout, rather than only ever seeing
// the immediate "unknown" answer.
func TestWaitForWriteOutcomeResolvesOnceAbandonedWriterFinishes(t *testing.T) {
	done := make(chan writeMutationResult, 1)
	err := &writeOutcomeUnknownError{ctxErr: context.Canceled, done: done}

	want := writeMutationResult{mutated: true, err: errors.New("disk quota exceeded")}
	go func() {
		time.Sleep(20 * time.Millisecond)
		done <- want
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mutated, resultErr, resolved := WaitForWriteOutcome(ctx, err)
	assert.True(t, resolved, "the abandoned writer's real result must be observed once it lands within the timeout")
	assert.True(t, mutated)
	assert.Equal(t, want.err, resultErr)
}

// With both channels ready, completed results must win over an expired wait.
func TestWaitForWriteOutcomePrefersBufferedResultOverExpiredTimer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	done := make(chan writeMutationResult, 1)
	want := writeMutationResult{mutated: true, err: errors.New("disk quota exceeded")}
	done <- want
	err := &writeOutcomeUnknownError{ctxErr: context.Canceled, done: done}

	const iterations = 200
	for i := 0; i < iterations; i++ {
		// Re-populate done for every iteration except the first, whose
		// value was already consumed by the previous iteration's call.
		if i > 0 {
			done <- want
		}
		mutated, resultErr, resolved := WaitForWriteOutcome(ctx, err)
		require.True(t, resolved, "iteration %d: a completed result already buffered in done must never be reported as an expired wait, regardless of which select case Go's runtime happened to pick", i)
		assert.True(t, mutated, "iteration %d", i)
		assert.Equal(t, want.err, resultErr, "iteration %d", i)
	}
}

// TestWaitForWriteOutcomeGivesUpAfterTimeout is a regression test for the
// same P1 finding's other half: if the abandoned writer has not finished
// within the given timeout, WaitForWriteOutcome must give up and report
// the caller's original, unresolved situation — mutated=true (still
// conservative) and the original write-outcome-unknown error unchanged —
// rather than blocking indefinitely.
func TestWaitForWriteOutcomeGivesUpAfterTimeout(t *testing.T) {
	done := make(chan writeMutationResult, 1)
	err := &writeOutcomeUnknownError{ctxErr: context.Canceled, done: done}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	mutated, resultErr, resolved := WaitForWriteOutcome(ctx, err)
	assert.False(t, resolved, "a writer that never finishes within the timeout must not be reported as resolved")
	assert.True(t, mutated, "still conservative: the abandoned write may yet land bytes after this call gives up")
	assert.Same(t, err, resultErr, "the original error must be returned unchanged when the wait times out")
}

func TestWaitForWriteOutcomeHonorsExplicitCancellation(t *testing.T) {
	done := make(chan writeMutationResult)
	err := &writeOutcomeUnknownError{ctxErr: context.DeadlineExceeded, done: done}
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan bool, 1)
	go func() {
		_, _, resolved := WaitForWriteOutcome(ctx, err)
		returned <- resolved
	}()
	cancel()
	select {
	case resolved := <-returned:
		require.False(t, resolved)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop outcome resolution")
	}
}

// TestWaitForWriteOutcomePanicsOnWrongErrorType pins WaitForWriteOutcome's
// documented contract: it must only ever be called on an error that is
// (or wraps) a writeOutcomeUnknownError, since any other error has no
// done channel to wait on at all. Calling it on anything else is a
// programming error at the call site, not a runtime condition a caller
// should need to handle.
func TestWaitForWriteOutcomePanicsOnWrongErrorType(t *testing.T) {
	assert.Panics(t, func() {
		WaitForWriteOutcome(context.Background(), errors.New("not a write-outcome-unknown error"))
	})
}

// TestPreferCompletedWriteResultReturnsBufferedResult is a regression test
// for a P1 finding: writeAndTruncateBounded's ctx.Done() branch could win
// Go's select over an already-complete result sitting in done (select
// makes no guarantee about which ready case is chosen), converting a
// known, safely restorable write outcome into a spurious
// ErrWriteOutcomeUnknown. preferCompletedWriteResult is the extracted
// recheck that must always prefer a buffered result when one is present,
// tested directly here rather than end-to-end: constructing a real
// scenario where ctx becomes done and the goroutine's send on done are
// simultaneously ready, without either one deterministically preceding
// the other, is not something a black-box test can reliably force — there
// is always some real work (Truncate, Close, struct construction, channel
// scheduling) between any injectable trigger point and the actual send,
// during which ctx.Done() can legitimately (and correctly) become
// observable to the caller first. Testing the extracted recheck directly
// against a pre-populated channel exercises the actual fix deterministically
// instead.
func TestPreferCompletedWriteResultReturnsBufferedResult(t *testing.T) {
	done := make(chan writeMutationResult, 1)
	want := writeMutationResult{mutated: true, err: errors.New("no space left on device")}
	done <- want

	got, ok := preferCompletedWriteResult(done)
	require.True(t, ok, "a result already sitting in the channel must be reported as completed, not treated as abandoned")
	assert.Equal(t, want, got)
}

// TestPreferCompletedWriteResultReportsNoneWhenEmpty verifies the
// converse: an empty channel (a genuinely still-running write, not yet
// finished) must be reported as not-yet-completed rather than blocking or
// fabricating a result.
func TestPreferCompletedWriteResultReportsNoneWhenEmpty(t *testing.T) {
	done := make(chan writeMutationResult, 1)
	_, ok := preferCompletedWriteResult(done)
	assert.False(t, ok, "an empty channel must be reported as not yet completed")
}

// TestPreferCompletedAcquisitionReturnsBufferedResult is
// preferCompletedAcquisition's counterpart for
// raceAcquisitionAgainstContext's own identical select race — see
// TestPreferCompletedWriteResultReturnsBufferedResult's doc for the full
// rationale this mirrors.
func TestPreferCompletedAcquisitionReturnsBufferedResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	f, err := os.Create(path)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })

	done := make(chan acquisitionResult, 1)
	want := acquisitionResult{f: f, err: nil}
	done <- want

	got, ok := preferCompletedAcquisition(done)
	require.True(t, ok, "a result already sitting in the channel must be reported as completed, not treated as abandoned")
	assert.Equal(t, want, got)
}

// TestPreferCompletedAcquisitionReportsNoneWhenEmpty verifies the
// converse: an empty channel must be reported as not yet completed.
func TestPreferCompletedAcquisitionReportsNoneWhenEmpty(t *testing.T) {
	done := make(chan acquisitionResult, 1)
	_, ok := preferCompletedAcquisition(done)
	assert.False(t, ok, "an empty channel must be reported as not yet completed")
}

// cancelAfterNCalls wraps a context.Context and calls its own cancel func
// the Nth time Err() is called, then delegates to the wrapped context —
// simulating a deadline/cancellation that arrives partway through a
// multi-chunk write, deterministically, without a real wall-clock race.
type cancelAfterNCalls struct {
	context.Context
	cancel      context.CancelFunc
	cancelAfter int
	calls       int
}

func (c *cancelAfterNCalls) Err() error {
	c.calls++
	if c.calls == c.cancelAfter {
		c.cancel()
	}
	return c.Context.Err()
}

// TestSandboxWriteRegularFileAcceptsMatchingIdentity verifies that passing
// the actual pre-write fs.FileInfo of the target as expectedIdentity does
// not itself block a legitimate, unmodified write.
func TestSandboxWriteRegularFileAcceptsMatchingIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0644))

	info, err := os.Stat(path)
	require.NoError(t, err)

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), "data.txt", dir, []byte("updated"), info)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "updated", string(got))
}

// TestSandboxWriteRegularFileRejectsIdentityMismatch is the P2 regression
// test: a caller that read one file's content and computed replacement
// bytes from it must not have those bytes written into a *different* file
// that has since been swapped into the same path — even though that
// replacement file is, by itself, an entirely ordinary, single-linked,
// regular file that the type check alone would happily accept.
func TestSandboxWriteRegularFileRejectsIdentityMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0644))

	// Capture the identity of the original file, then replace it (as a
	// distinct file, not an in-place edit of the same inode) with a new,
	// otherwise perfectly acceptable regular file before the write-back —
	// simulating another process swapping the path between an earlier read
	// and this write.
	// On Windows, os.Stat defers file-ID lookup until os.SameFile; use a
	// descriptor to capture the original ID before replacing the path.
	original, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { original.Close() })
	originalInfo, err := original.Stat()
	require.NoError(t, err)
	require.NoError(t, original.Close()) // allow replacement on Windows
	replacement := filepath.Join(dir, "replacement.txt")
	require.NoError(t, os.WriteFile(replacement, []byte("someone else's file"), 0644))
	require.NoError(t, os.Rename(replacement, path))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), "data.txt", dir, []byte("attacker-derived content"), originalInfo)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file identity changed")

	// The swapped-in file must be completely untouched.
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "someone else's file", string(got))
}

// TestSandboxWriteRegularFileSkipsIdentityCheckWhenNil verifies that a nil
// expectedIdentity (the pre-existing behaviour, used by callers with no
// prior read to pin against) still performs the write unconditionally.
func TestSandboxWriteRegularFileSkipsIdentityCheckWhenNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), "data.txt", dir, []byte("new"), nil)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func TestSandboxRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	require.NoError(t, sb.Remove("data.txt", dir))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "file should be removed")
}

func TestSandboxRemoveReadOnlyRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	// No SetWritable() — sandbox defaults to read-only.

	err = sb.Remove("data.txt", dir)
	assert.ErrorIs(t, err, os.ErrPermission)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "file must survive a rejected remove")
}

func TestSandboxRemoveOutsideAllowedPathRejected(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(target, []byte("secret"), 0644))

	sb, _, err := New([]string{allowed + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove(target, allowed)
	assert.ErrorIs(t, err, os.ErrPermission)
	_, statErr := os.Stat(target)
	assert.NoError(t, statErr, "file outside the allowlist must survive")
}

func TestSandboxRemoveReadOnlyRootRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0644))

	// Root configured read-only (":ro", or bare with no suffix) rejects
	// removal even though the sandbox overall is writable.
	sb, _, err := New([]string{dir + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove("data.txt", dir)
	assert.ErrorIs(t, err, os.ErrPermission)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestSandboxRemoveDirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(sub, 0755))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove("subdir", dir)
	assert.Error(t, err)
	_, statErr := os.Stat(sub)
	assert.NoError(t, statErr, "directory must survive a rejected remove")
}

// TestSandboxRemoveTrailingSlashOnFileRejected verifies the trailing-dir-syntax
// enforcement lives in Sandbox.Remove itself, not only in rm.go's own
// precheck: calling Remove directly with a path syntactically requiring a
// directory (a trailing "/") against a plain file must fail with "not a
// directory" rather than removing the file.
func TestSandboxRemoveTrailingSlashOnFileRejected(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("keep me"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove("file.txt/", dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
	_, statErr := os.Stat(file)
	assert.NoError(t, statErr, "file must survive a rejected remove")
}

// TestSandboxRemoveTrailingSlashOnSymlinkToDirRejected verifies that a
// trailing "/" forces dereferencing a symlink leaf: removing a
// symlink-to-directory via Sandbox.Remove with a trailing slash must fail
// with "is a directory" (matching POSIX unlink(2) semantics for a path with
// a trailing separator), not silently remove the symlink itself.
func TestSandboxRemoveTrailingSlashOnSymlinkToDirRejected(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(sub, 0755))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink("subdir", link))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove("link/", dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
	_, lstatErr := os.Lstat(link)
	assert.NoError(t, lstatErr, "symlink must survive a rejected remove")
}

// TestSandboxRemoveTrailingSlashOnEscapingSymlinkRejected verifies that a
// trailing "/" on a same-root symlink whose target lies outside every
// configured AllowedPaths root is rejected with a permission error before
// Unlink ever dereferences it. Without this check, the trailing-slash
// dereference (required to distinguish "is a directory" from "not a
// directory") would issue a real stat syscall against the out-of-sandbox
// target, leaking its existence/type — a sandbox-boundary leak, since the
// symlink itself resolves fine but its referent was never validated.
func TestSandboxRemoveTrailingSlashOnEscapingSymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(outside, "secretdir"), 0755))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(filepath.Join(outside, "secretdir"), link))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove("link/", dir)
	assert.Error(t, err)
	assert.ErrorIs(t, err, os.ErrPermission)
	_, lstatErr := os.Lstat(link)
	assert.NoError(t, lstatErr, "symlink must survive a rejected remove")
}

func TestSandboxRemoveMissingFile(t *testing.T) {
	dir := t.TempDir()

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove("missing.txt", dir)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// TestSandboxRemoveSymlinkRemovesLinkNotTarget verifies unlink(2) semantics:
// the symlink itself is deleted, its referent is left untouched. This is the
// key difference between Remove and Truncate/Open's write-target resolution,
// which reject the final component being a symlink.
func TestSandboxRemoveSymlinkRemovesLinkNotTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("keep me"), 0644))
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink("target.txt", link))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	require.NoError(t, sb.Remove("link.txt", dir))

	_, linkErr := os.Lstat(link)
	assert.True(t, os.IsNotExist(linkErr), "symlink should be removed")
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(got), "symlink target must survive")
}

func TestSandboxRemoveDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	require.NoError(t, os.Symlink("does-not-exist", link))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	require.NoError(t, sb.Remove("dangling", dir))
	_, linkErr := os.Lstat(link)
	assert.True(t, os.IsNotExist(linkErr))
}

// TestSandboxRemoveSymlinkTargetOutsideSandboxStillRemovable regression-tests
// the bug in an earlier version of Remove that reused resolveWriteTarget,
// which follows the final path component. unlink(2) never dereferences the
// final component, so a symlink whose target lies outside every allowed
// root (or doesn't exist) must still be removable by name — Remove must not
// require its target to resolve anywhere.
func TestSandboxRemoveSymlinkTargetOutsideSandboxStillRemovable(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(dir, "escape_link")
	require.NoError(t, os.Symlink(filepath.Join(outside, "does-not-exist"), link))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	require.NoError(t, sb.Remove("escape_link", dir))
	_, lstatErr := os.Lstat(link)
	assert.True(t, os.IsNotExist(lstatErr))
}

// TestSandboxRemoveSelfReferentialSymlinkNoHang regression-tests the bug in
// an earlier version of Remove that reused resolveWriteTarget: following a
// self-referential symlink ("loop" -> "loop") during resolution either
// errors out via ELOOP or, in the worst case, could hang. Remove must
// succeed immediately since unlink(2) never follows the final component.
func TestSandboxRemoveSelfReferentialSymlinkNoHang(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "loop")
	require.NoError(t, os.Symlink("loop", link))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	require.NoError(t, sb.Remove("loop", dir))
	_, lstatErr := os.Lstat(link)
	assert.True(t, os.IsNotExist(lstatErr))
}

// TestSandboxRemoveThroughSymlinkedIntermediateDirRejected ensures a symlink
// used as an intermediate path component (not the final component) cannot be
// used to escape the sandbox root, mirroring the write-target protection in
// resolveWriteTarget/rejectSymlinkWriteTarget.
func TestSandboxRemoveThroughSymlinkedIntermediateDirRejected(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "victim.txt")
	require.NoError(t, os.WriteFile(target, []byte("victim"), 0644))

	linkDir := filepath.Join(allowed, "linkdir")
	require.NoError(t, os.Symlink(outside, linkDir))

	sb, _, err := New([]string{allowed + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove(filepath.Join(allowed, "linkdir", "victim.txt"), allowed)
	assert.Error(t, err)
	_, statErr := os.Stat(target)
	assert.NoError(t, statErr, "file behind a symlinked intermediate directory must survive")
}

// TestSandboxRemoveThroughSymlinkIntoNestedReadOnlyRootRejected reproduces
// the scenario raised in review: a broad :rw parent root containing a more
// specific, nested :ro child root. If Remove followed a raced intermediate
// symlink pointing from the :rw region into the :ro region before deleting,
// it would delete a file the operator scoped as read-only. Remove's
// no-follow openat walk (writeopen.Unlink) rejects the symlinked
// intermediate component outright, regardless of which root it resolves
// into, so the file inside the nested read-only root must survive.
func TestSandboxRemoveThroughSymlinkIntoNestedReadOnlyRootRejected(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "readonly-child")
	require.NoError(t, os.Mkdir(child, 0755))
	victim := filepath.Join(child, "victim.txt")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0644))

	link := filepath.Join(parent, "linkdir")
	require.NoError(t, os.Symlink(child, link))

	sb, _, err := New([]string{parent + ":rw", child + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Remove(filepath.Join(parent, "linkdir", "victim.txt"), parent)
	assert.Error(t, err)
	_, statErr := os.Stat(victim)
	assert.NoError(t, statErr, "file inside the nested read-only root must survive a remove through a symlinked intermediate")
}

// TestSandboxWriteThroughSymlinkEscapeRejected ensures the cross-root
// symlink fallback is not used for write opens. Following a symlink that
// escapes its os.Root and then performing a create or truncate is the
// classic TOCTOU footgun: the link target can be swapped between
// resolution and open. Writes must stay inside a single os.Root.
func TestSandboxWriteThroughSymlinkEscapeRejected(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	// Symlink inside the allowlist that points to a path *outside* the
	// allowlist. os.Root will reject this with a path-escape error;
	// the sandbox must then refuse to fall back for writes.
	linkPath := filepath.Join(allowed, "escape")
	require.NoError(t, os.Symlink(filepath.Join(outside, "target.txt"), linkPath))

	sb, _, err := New([]string{allowed + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	f, err := sb.Open("escape", allowed, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	assert.Nil(t, f)
	assert.Error(t, err)

	_, statErr := os.Stat(filepath.Join(outside, "target.txt"))
	assert.True(t, os.IsNotExist(statErr), "symlink target must not be created outside the sandbox")
}

func TestSandboxWriteRejectsUnknownFlag(t *testing.T) {
	dir := t.TempDir()

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	// Pick a high bit that is not in allowedOpenFlags. We OR with
	// O_WRONLY so the access mode itself is valid; only the unknown bit
	// should trigger rejection.
	const unknownFlag = 1 << 30
	f, err := sb.Open("x.txt", dir, os.O_WRONLY|os.O_CREATE|unknownFlag, 0644)
	assert.Nil(t, f)
	assert.ErrorIs(t, err, os.ErrPermission)
}

func TestSandboxReadOpens(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	f, err := sb.Open("test.txt", dir, os.O_RDONLY, 0)
	require.NoError(t, err)
	data, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
	require.NoError(t, f.Close())

	handle, err := sb.OpenRegular("test.txt", dir)
	require.NoError(t, err)
	defer handle.Close()
	regularData, err := io.ReadAll(handle)
	require.NoError(t, err)
	assert.Equal(t, "data", string(regularData))
}

func TestParseAllowedPathMode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		path string
		mode pathMode
	}{
		{name: "default read-only", in: "/var/log", path: "/var/log", mode: pathModeReadOnly},
		{name: "explicit read-only", in: "/var/log:ro", path: "/var/log", mode: pathModeReadOnly},
		{name: "explicit read-write", in: "/var/log:rw", path: "/var/log", mode: pathModeReadWrite},
		{name: "last terminal suffix wins", in: "/var/log:rw:ro", path: "/var/log:rw", mode: pathModeReadOnly},
		{name: "middle suffix is path text", in: "/var/log:rw/datadog", path: "/var/log:rw/datadog", mode: pathModeReadOnly},
		{name: "unknown suffix is path text", in: "/var/log:rx", path: "/var/log:rx", mode: pathModeReadOnly},
		{name: "bare ro suffix is path text", in: ":ro", path: ":ro", mode: pathModeReadOnly},
		{name: "bare rw suffix is path text", in: ":rw", path: ":rw", mode: pathModeReadOnly},
		{name: "empty path", in: "", path: "", mode: pathModeReadOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, mode := parseAllowedPathMode(tt.in)
			assert.Equal(t, tt.path, path)
			assert.Equal(t, tt.mode, mode)
		})
	}
}

func TestResolveAllowedPathModePreservesExistingLiteralPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("literal paths ending in :rw/:ro are POSIX-only")
	}

	dir := t.TempDir()
	literal := filepath.Join(dir, "tenant:rw")
	require.NoError(t, os.Mkdir(literal, 0755))

	path, mode := resolveAllowedPathMode(literal)
	assert.Equal(t, literal, path)
	assert.Equal(t, pathModeReadOnly, mode)
}

func TestAllowedPathModesAreStoredAfterSuffixStripping(t *testing.T) {
	dir := t.TempDir()

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()

	assert.Equal(t, []string{dir}, sb.Paths())

	root, _, ok := sb.resolve(dir)
	require.True(t, ok)
	assert.Equal(t, pathModeReadWrite, root.mode)
}

func TestPathAccessesIncludeModes(t *testing.T) {
	readOnly := t.TempDir()
	readWrite := t.TempDir()

	sb, _, err := New([]string{readOnly, readWrite + ":rw"})
	require.NoError(t, err)
	defer sb.Close()

	assert.Equal(t, []PathAccess{
		{Path: readOnly, ReadWrite: false},
		{Path: readWrite, ReadWrite: true},
	}, sb.PathAccesses())
}

func TestAllowedPathModeMostSpecificRootWins(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "datadog")
	require.NoError(t, os.Mkdir(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(child, "agent.log"), []byte("data"), 0644))

	sb, _, err := New([]string{dir + ":rw", child + ":ro"})
	require.NoError(t, err)
	defer sb.Close()

	root, _, ok := sb.resolve(filepath.Join(child, "agent.log"))
	require.True(t, ok)
	assert.Equal(t, child, root.absPath)
	assert.Equal(t, pathModeReadOnly, root.mode)
}

func TestAllowedPathModeMostSpecificReadWriteWins(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "datadog")
	require.NoError(t, os.Mkdir(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(child, "agent.log"), []byte("data"), 0644))

	sb, _, err := New([]string{dir + ":ro", child + ":rw"})
	require.NoError(t, err)
	defer sb.Close()

	root, _, ok := sb.resolve(filepath.Join(child, "agent.log"))
	require.True(t, ok)
	assert.Equal(t, child, root.absPath)
	assert.Equal(t, pathModeReadWrite, root.mode)
}

func TestAllowedPathReadWriteModeDoesNotEnableWriteOpen(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()

	f, err := sb.Open("test.txt", dir, os.O_RDWR, 0)
	assert.Nil(t, f)
	assert.ErrorIs(t, err, os.ErrPermission)
}

func TestAllowedPathReadOnlyModeRejectsWriteOpenInWritableSandbox(t *testing.T) {
	tests := []struct {
		name         string
		pathSuffix   string
		wantRootMode pathMode
	}{
		{name: "default read-only", wantRootMode: pathModeReadOnly},
		{name: "explicit read-only", pathSuffix: ":ro", wantRootMode: pathModeReadOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "test.txt")
			require.NoError(t, os.WriteFile(target, []byte("data"), 0644))

			sb, _, err := New([]string{dir + tt.pathSuffix})
			require.NoError(t, err)
			defer sb.Close()
			sb.SetWritable()

			root, _, ok := sb.resolve(target)
			require.True(t, ok)
			require.Equal(t, tt.wantRootMode, root.mode)

			f, err := sb.Open("test.txt", dir, os.O_WRONLY|os.O_TRUNC, 0)
			assert.Nil(t, f)
			assert.ErrorIs(t, err, os.ErrPermission)

			got, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, "data", string(got))
		})
	}
}

func TestAllowedPathMostSpecificReadOnlyModeRejectsWriteOpen(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0755))
	parentTarget := filepath.Join(parent, "parent.txt")
	childTarget := filepath.Join(child, "child.txt")
	require.NoError(t, os.WriteFile(parentTarget, []byte("parent"), 0644))
	require.NoError(t, os.WriteFile(childTarget, []byte("child"), 0644))

	sb, _, err := New([]string{parent + ":rw", child + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	childFile, err := sb.Open("child.txt", child, os.O_WRONLY|os.O_TRUNC, 0)
	assert.Nil(t, childFile)
	assert.ErrorIs(t, err, os.ErrPermission)

	parentFile, err := sb.Open("parent.txt", parent, os.O_WRONLY|os.O_TRUNC, 0)
	require.NoError(t, err)
	_, err = parentFile.Write([]byte("updated"))
	require.NoError(t, err)
	require.NoError(t, parentFile.Close())

	gotChild, err := os.ReadFile(childTarget)
	require.NoError(t, err)
	assert.Equal(t, "child", string(gotChild))
	gotParent, err := os.ReadFile(parentTarget)
	require.NoError(t, err)
	assert.Equal(t, "updated", string(gotParent))
}

func TestAllowedPathReadOnlyModeRejectsTruncateInWritableSandbox(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0644))

	sb, _, err := New([]string{dir + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Truncate("test.txt", dir, 0, false)
	assert.ErrorIs(t, err, os.ErrPermission)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "data", string(got))
}

func TestAllowedPathReadWriteModeAllowsTruncateInWritableSandbox(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	require.NoError(t, sb.Truncate("test.txt", dir, 2, false))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "da", string(got))
}

func TestAllowedPathModeDoesNotWidenExistingLiteralSuffixPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("literal paths ending in :rw/:ro are POSIX-only")
	}

	parent := t.TempDir()
	base := filepath.Join(parent, "tenant")
	literal := base + ":rw"
	require.NoError(t, os.Mkdir(base, 0755))
	require.NoError(t, os.Mkdir(literal, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "base.txt"), []byte("base"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(literal, "literal.txt"), []byte("literal"), 0644))

	sb, _, err := New([]string{literal})
	require.NoError(t, err)
	defer sb.Close()

	assert.Equal(t, []string{literal}, sb.Paths())

	root, _, ok := sb.resolve(filepath.Join(literal, "literal.txt"))
	require.True(t, ok)
	assert.Equal(t, literal, root.absPath)
	assert.Equal(t, pathModeReadOnly, root.mode)

	f, err := sb.Open(filepath.Join(literal, "literal.txt"), "/", os.O_RDONLY, 0)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	_, err = sb.Open(filepath.Join(base, "base.txt"), "/", os.O_RDONLY, 0)
	assert.ErrorIs(t, err, os.ErrPermission)
}

func TestReadDirLimited(t *testing.T) {
	dir := t.TempDir()

	// Create 10 files.
	for i := range 10 {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d", i)), nil, 0644))
	}

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	t.Run("maxRead below count returns truncated with first N entries", func(t *testing.T) {
		entries, truncated, err := sb.ReadDirLimited(".", dir, 0, 5)
		require.NoError(t, err)
		assert.True(t, truncated)
		assert.Len(t, entries, 5)
		// Entries are sorted within the read window.
		for i := 1; i < len(entries); i++ {
			assert.True(t, entries[i-1].Name() < entries[i].Name(), "entries should be sorted")
		}
	})

	t.Run("maxRead above count returns all entries not truncated", func(t *testing.T) {
		entries, truncated, err := sb.ReadDirLimited(".", dir, 0, 20)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Len(t, entries, 10)
	})

	t.Run("empty directory", func(t *testing.T) {
		emptyDir := filepath.Join(dir, "empty")
		require.NoError(t, os.Mkdir(emptyDir, 0755))

		entries, truncated, err := sb.ReadDirLimited("empty", dir, 0, 10)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Empty(t, entries)
	})

	t.Run("path outside sandbox returns permission error", func(t *testing.T) {
		outsideDir := t.TempDir()
		_, _, err := sb.ReadDirLimited(outsideDir, dir, 0, 10)
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrPermission)
	})

	t.Run("io.EOF is not returned as error", func(t *testing.T) {
		// Use a fresh directory to avoid interference from other subtests.
		eofDir := filepath.Join(dir, "eoftest")
		require.NoError(t, os.Mkdir(eofDir, 0755))
		for i := range 5 {
			require.NoError(t, os.WriteFile(filepath.Join(eofDir, fmt.Sprintf("g%02d", i)), nil, 0644))
		}
		entries, truncated, err := sb.ReadDirLimited("eoftest", dir, 0, 1000)
		require.NoError(t, err, "io.EOF should not be returned as error")
		assert.False(t, truncated)
		assert.Len(t, entries, 5)
	})

	t.Run("non-positive maxRead returns empty", func(t *testing.T) {
		entries, truncated, err := sb.ReadDirLimited(".", dir, 0, 0)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Empty(t, entries)

		entries, truncated, err = sb.ReadDirLimited(".", dir, 0, -5)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Empty(t, entries)
	})

	t.Run("offset skips entries", func(t *testing.T) {
		// Use a fresh directory to avoid interference from other subtests.
		offsetDir := filepath.Join(dir, "offsettest")
		require.NoError(t, os.Mkdir(offsetDir, 0755))
		for i := range 10 {
			require.NoError(t, os.WriteFile(filepath.Join(offsetDir, fmt.Sprintf("h%02d", i)), nil, 0644))
		}

		// Read all 10 entries with no offset for reference.
		all, _, err := sb.ReadDirLimited("offsettest", dir, 0, 100)
		require.NoError(t, err)
		assert.Len(t, all, 10)

		// Skip first 5 entries, read up to 100.
		entries, truncated, err := sb.ReadDirLimited("offsettest", dir, 5, 100)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Len(t, entries, 5, "should return remaining 5 entries after skipping 5")
	})

	t.Run("offset beyond count returns empty", func(t *testing.T) {
		entries, truncated, err := sb.ReadDirLimited("offsettest", dir, 100, 10)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Empty(t, entries)
	})

	t.Run("offset plus maxRead with truncation", func(t *testing.T) {
		// Skip 3, read 3 out of 10 => should get 3 entries, truncated.
		entries, truncated, err := sb.ReadDirLimited("offsettest", dir, 3, 3)
		require.NoError(t, err)
		assert.True(t, truncated, "should be truncated since 10 - 3 > 3")
		assert.Len(t, entries, 3)
		// Entries should be sorted within the window.
		for i := 1; i < len(entries); i++ {
			assert.True(t, entries[i-1].Name() < entries[i].Name(), "entries should be sorted")
		}
	})

	t.Run("negative offset clamped to zero", func(t *testing.T) {
		entries, truncated, err := sb.ReadDirLimited("offsettest", dir, -10, 100)
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Len(t, entries, 10, "negative offset should be treated as 0")
	})
}

func TestReadDirNCapExceeded(t *testing.T) {
	dir := t.TempDir()

	// Create 4 files so the directory exceeds a cap of 3.
	for i := range 4 {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d", i)), nil, 0644))
	}

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	t.Run("returns error when entries exceed cap", func(t *testing.T) {
		entries, err := sb.readDirN(".", dir, 3)
		assert.Nil(t, entries)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many entries")
	})

	t.Run("returns entries when count equals cap", func(t *testing.T) {
		entries, err := sb.readDirN(".", dir, 4)
		require.NoError(t, err)
		assert.Len(t, entries, 4)
	})

	t.Run("returns entries when count is below cap", func(t *testing.T) {
		entries, err := sb.readDirN(".", dir, 10)
		require.NoError(t, err)
		assert.Len(t, entries, 4)
	})
}

func TestCollectDirEntries(t *testing.T) {
	makeEntries := func(names ...string) []fs.DirEntry {
		out := make([]fs.DirEntry, len(names))
		for i, n := range names {
			out[i] = fakeDirEntry{name: n}
		}
		return out
	}

	t.Run("error in same batch as truncation is preserved", func(t *testing.T) {
		ioErr := errors.New("disk I/O error")
		callCount := 0
		reader := func(n int) ([]fs.DirEntry, error) {
			callCount++
			if callCount == 1 {
				return makeEntries("f01", "f02", "f03", "f04", "f05"), nil
			}
			return makeEntries("f06", "f07", "f08"), ioErr
		}

		entries, truncated, err := CollectDirEntries(reader, 10, 0, 6)
		assert.True(t, truncated, "should be truncated")
		assert.Len(t, entries, 6, "should trim to maxRead")
		assert.ErrorIs(t, err, ioErr, "I/O error must be preserved even when truncation occurs")
	})

	t.Run("EOF is not returned as error", func(t *testing.T) {
		callCount := 0
		reader := func(n int) ([]fs.DirEntry, error) {
			callCount++
			if callCount == 1 {
				return makeEntries("f01", "f02"), io.EOF
			}
			return nil, io.EOF
		}

		entries, truncated, err := CollectDirEntries(reader, 10, 0, 100)
		assert.False(t, truncated)
		assert.Len(t, entries, 2)
		assert.NoError(t, err, "io.EOF should not be returned as error")
	})

	t.Run("offset skips entries across batches", func(t *testing.T) {
		callCount := 0
		reader := func(n int) ([]fs.DirEntry, error) {
			callCount++
			if callCount == 1 {
				return makeEntries("f01", "f02", "f03"), nil
			}
			if callCount == 2 {
				return makeEntries("f04", "f05"), io.EOF
			}
			return nil, io.EOF
		}

		entries, truncated, err := CollectDirEntries(reader, 10, 2, 100)
		assert.False(t, truncated)
		assert.NoError(t, err)
		assert.Len(t, entries, 3, "should skip first 2, return remaining 3")
		assert.Equal(t, "f03", entries[0].Name())
		assert.Equal(t, "f04", entries[1].Name())
		assert.Equal(t, "f05", entries[2].Name())
	})

	t.Run("error without truncation is preserved", func(t *testing.T) {
		ioErr := errors.New("permission denied")
		reader := func(n int) ([]fs.DirEntry, error) {
			return makeEntries("f01", "f02"), ioErr
		}

		entries, truncated, err := CollectDirEntries(reader, 10, 0, 100)
		assert.False(t, truncated)
		assert.Len(t, entries, 2)
		assert.ErrorIs(t, err, ioErr)
	})

	t.Run("entries are sorted by name", func(t *testing.T) {
		reader := func(n int) ([]fs.DirEntry, error) {
			return makeEntries("cherry", "apple", "banana"), io.EOF
		}

		entries, truncated, err := CollectDirEntries(reader, 10, 0, 100)
		assert.False(t, truncated)
		assert.NoError(t, err)
		assert.Equal(t, "apple", entries[0].Name())
		assert.Equal(t, "banana", entries[1].Name())
		assert.Equal(t, "cherry", entries[2].Name())
	})
}

func TestNewSkipsNonexistentPaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("data"), 0644))

	sb, _, err := New([]string{"/nonexistent/path", dir})
	require.NoError(t, err, "nonexistent paths should be skipped")
	defer sb.Close()

	// The existing directory should still be accessible.
	f, err := sb.Open(filepath.Join(dir, "test.txt"), dir, os.O_RDONLY, 0)
	require.NoError(t, err)
	f.Close()
}

func TestNewAllPathsNonexistent(t *testing.T) {
	sb, _, err := New([]string{"/does/not/exist", "/also/missing"})
	require.NoError(t, err, "all-nonexistent paths should succeed with empty sandbox")
	defer sb.Close()

	// Sandbox should block all access.
	_, err = sb.Stat("/tmp", "/tmp")
	assert.ErrorIs(t, err, os.ErrPermission)
}

func TestNewEmptyPaths(t *testing.T) {
	sb, _, err := New([]string{})
	require.NoError(t, err, "empty path list should succeed")
	defer sb.Close()

	// Sandbox should block all access.
	_, err = sb.Stat("/tmp", "/tmp")
	assert.ErrorIs(t, err, os.ErrPermission)
}

func TestNewMixedExistingAndNonexistent(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dirA, "a.txt"), []byte("aaa"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dirB, "b.txt"), []byte("bbb"), 0644))

	sb, _, err := New([]string{dirA, "/nonexistent", dirB})
	require.NoError(t, err, "nonexistent path between valid dirs should be skipped")
	defer sb.Close()

	// Both existing directories should be accessible.
	f, err := sb.Open(filepath.Join(dirA, "a.txt"), dirA, os.O_RDONLY, 0)
	require.NoError(t, err, "first existing dir should work")
	f.Close()

	f, err = sb.Open(filepath.Join(dirB, "b.txt"), dirB, os.O_RDONLY, 0)
	require.NoError(t, err, "second existing dir should work")
	f.Close()
}
