// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths/internal/writeopen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestAccessFIFODoesNotBlock verifies that Access on a FIFO (named pipe) with
// no writer returns immediately instead of blocking. Before the fix, Access
// used os.Root.Open which blocks on FIFOs until a writer appears.
func TestAccessFIFODoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "pipe")
	require.NoError(t, syscall.Mkfifo(fifoPath, 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	done := make(chan error, 1)
	go func() {
		done <- sb.Access("pipe", dir, 0x04) // read check
	}()

	select {
	case err := <-done:
		// Should succeed (file exists and is readable) without blocking.
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Access blocked on FIFO — expected non-blocking stat-based check")
	}
}

func TestReadOnlyAllowedPathDoesNotOpenWriteRoot(t *testing.T) {
	dir := t.TempDir()

	sb, warnings, err := New([]string{dir, dir + ":ro"})
	require.NoError(t, err)
	require.Empty(t, warnings)
	defer sb.Close()
	require.Len(t, sb.roots, 2)

	assert.Nil(t, sb.roots[0].writeRoot)
	assert.Nil(t, sb.roots[1].writeRoot)
}

func TestReadWriteAllowedPathOpensWriteRoot(t *testing.T) {
	dir := t.TempDir()

	sb, warnings, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	require.Empty(t, warnings)
	defer sb.Close()
	require.Len(t, sb.roots, 1)

	require.NotNil(t, sb.roots[0].writeRoot)
	readInfo, err := sb.roots[0].root.Stat(".")
	require.NoError(t, err)
	writeInfo, err := sb.roots[0].writeRoot.Stat()
	require.NoError(t, err)
	assert.True(t, os.SameFile(readInfo, writeInfo))
}

func TestCanonicalForOpenedRootAcceptsMatchingSymlink(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	linkDir := filepath.Join(dir, "link")
	require.NoError(t, os.Mkdir(realDir, 0o755))
	require.NoError(t, os.Symlink(realDir, linkDir))

	root, err := os.OpenRoot(linkDir)
	require.NoError(t, err)
	defer root.Close()

	canonical, err := canonicalForOpenedRoot(linkDir, root)
	require.NoError(t, err)
	canonicalInfo, err := os.Stat(canonical)
	require.NoError(t, err)
	realInfo, err := os.Stat(realDir)
	require.NoError(t, err)
	assert.True(t, os.SameFile(canonicalInfo, realInfo))
}

func TestCanonicalForOpenedRootRejectsMismatchedPath(t *testing.T) {
	dir := t.TempDir()
	openedDir := filepath.Join(dir, "opened")
	otherDir := filepath.Join(dir, "other")
	require.NoError(t, os.Mkdir(openedDir, 0o755))
	require.NoError(t, os.Mkdir(otherDir, 0o755))

	root, err := os.OpenRoot(openedDir)
	require.NoError(t, err)
	defer root.Close()

	_, err = canonicalForOpenedRoot(otherDir, root)
	require.Error(t, err)
}

// TestAccessReadPermissionDenied verifies that Access returns an error for
// files that are not readable by the current user.
func TestAccessReadPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noread.txt"), []byte("secret"), 0o200))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access("noread.txt", dir, 0x04)
	assert.ErrorIs(t, err, os.ErrPermission)
}

// TestAccessWriteDenied verifies that Access returns an error for files
// that are not writable by the current user (mode 0444).
func TestAccessWriteDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readonly.txt"), []byte("data"), 0o444))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access("readonly.txt", dir, 0x02)
	assert.ErrorIs(t, err, os.ErrPermission)
}

// TestAccessExecDenied verifies that Access returns an error for files
// that are not executable by the current user (mode 0644).
func TestAccessExecDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noexec.txt"), []byte("data"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access("noexec.txt", dir, 0x01)
	assert.ErrorIs(t, err, os.ErrPermission)
}

// TestAccessReadAllowed verifies that Access succeeds for a readable file.
func TestAccessReadAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("data"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("readable.txt", dir, 0x04))
}

// TestAccessWriteAllowed verifies that Access succeeds for a writable file.
func TestAccessWriteAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "writable.txt"), []byte("data"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("writable.txt", dir, 0x02))
}

// TestAccessExecAllowed verifies that Access succeeds for an executable file.
func TestAccessExecAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "script.sh"), []byte("#!/bin/sh"), 0o755))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("script.sh", dir, 0x01))
}

// TestAccessNonexistent verifies that Access fails for a missing file.
func TestAccessNonexistent(t *testing.T) {
	dir := t.TempDir()

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access("missing.txt", dir, 0x04)
	assert.Error(t, err)
}

// TestAccessOutsideSandbox verifies that Access fails for a path
// outside the sandbox.
func TestAccessOutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access(filepath.Join(outside, "secret.txt"), dir, 0x04)
	assert.ErrorIs(t, err, os.ErrPermission)
}

// TestAccessDirectory verifies that Access works on directories.
func TestAccessDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("subdir", dir, 0x04))
}

// TestAccessSymlinkWithinSandbox verifies that Access succeeds for a
// symlink that resolves to a target within the sandbox.
func TestAccessSymlinkWithinSandbox(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("data"), 0o644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link.txt")))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("link.txt", dir, 0x04))
}

// TestIsPathEscapeError is a regression test for isPathEscapeError.
// If Go changes the "path escapes from parent" error string in a future
// version, this test will fail and alert us to update the detection.
func TestIsPathEscapeError(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "file.txt"), []byte("data"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "file.txt"), filepath.Join(dir, "escape.txt")))

	r, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer r.Close()

	_, err = r.OpenFile("escape.txt", os.O_RDONLY, 0)
	require.Error(t, err)
	assert.True(t, isPathEscapeError(err), "expected path escape error, got: %v", err)

	// Non-escape errors should not match.
	_, err = r.OpenFile("nonexistent.txt", os.O_RDONLY, 0)
	require.Error(t, err)
	assert.False(t, isPathEscapeError(err), "ENOENT should not be a path escape error")
}

// TestAccessSymlinkEscapeBlocked verifies that Access blocks symlinks
// that resolve outside the sandbox.
func TestAccessSymlinkEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access("escape.txt", dir, 0x04)
	assert.Error(t, err)
}

// TestAccessCombinedModes verifies that Access correctly checks
// combined permission modes (read+write, read+exec, etc.).
func TestAccessCombinedModes(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rx.sh"), []byte("#!/bin/sh"), 0o555))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	// Read+exec should succeed on 0555 file.
	assert.NoError(t, sb.Access("rx.sh", dir, 0x04|0x01))

	// Write should fail on 0555 file.
	assert.ErrorIs(t, sb.Access("rx.sh", dir, 0x02), os.ErrPermission)

	// Read+write should fail on 0555 file.
	assert.ErrorIs(t, sb.Access("rx.sh", dir, 0x04|0x02), os.ErrPermission)
}

// TestAccessReadRegularFileOpenFile verifies that read access on a
// regular file uses the fd-relative OpenFile path (not syscall.Access).
// A file with 0200 (write-only) should be denied.
func TestAccessReadRegularFileOpenFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "writeonly.txt"), []byte("data"), 0o200))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	assert.ErrorIs(t, sb.Access("writeonly.txt", dir, 0x04), os.ErrPermission)
}

// TestAccessReadRegularFileAllowed verifies read succeeds on a
// readable regular file via the OpenFile path.
func TestAccessReadRegularFileAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("data"), 0o644))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	assert.NoError(t, sb.Access("readable.txt", dir, 0x04))
}

// TestAccessFIFOReadFallsBackToModeBits verifies that FIFOs do NOT
// use OpenFile (which would block) and instead use effectiveHasPerm.
// A readable FIFO (0644) should pass the mode-bit check.
func TestAccessFIFOReadFallsBackToModeBits(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	done := make(chan error, 1)
	go func() { done <- sb.Access("pipe", dir, 0x04) }()
	select {
	case err := <-done:
		assert.NoError(t, err) // readable FIFO should pass mode-bit check
	case <-time.After(2 * time.Second):
		t.Fatal("Access blocked on FIFO")
	}
}

// TestAccessFIFOReadDeniedModeBits verifies that a non-readable FIFO
// (0200) is correctly denied via mode-bit fallback.
func TestAccessFIFOReadDeniedModeBits(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o200))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	done := make(chan error, 1)
	go func() { done <- sb.Access("pipe", dir, 0x04) }()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, os.ErrPermission)
	case <-time.After(2 * time.Second):
		t.Fatal("Access blocked on FIFO")
	}
}

// TestAccessDirectoryReadUsesModeBits verifies that directory read
// checks use mode-bit fallback (not OpenFile, which returns a handle).
func TestAccessDirectoryReadUsesModeBits(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	assert.NoError(t, sb.Access("subdir", dir, 0x04))
}

// TestAccessDirectoryReadDenied verifies that a non-readable directory
// (0300) is denied via mode-bit inspection.
func TestAccessDirectoryReadDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "noread"), 0o300))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	assert.ErrorIs(t, sb.Access("noread", dir, 0x04), os.ErrPermission)
}

// TestAccessReadWriteCombined verifies combined read+write checks.
// Read uses OpenFile (ACL-accurate), write uses effectiveHasPerm.
func TestAccessReadWriteCombined(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := t.TempDir()
	// 0444 = readable but not writable
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readonly.txt"), []byte("data"), 0o444))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	// Read-only succeeds
	assert.NoError(t, sb.Access("readonly.txt", dir, 0x04))
	// Write fails
	assert.ErrorIs(t, sb.Access("readonly.txt", dir, 0x02), os.ErrPermission)
	// Read+write fails (write component fails)
	assert.ErrorIs(t, sb.Access("readonly.txt", dir, 0x04|0x02), os.ErrPermission)
}

// TestAccessFdRelativeSymlink verifies that the permission check stays
// fd-relative. Access through a symlink within the sandbox works because
// both Stat and OpenFile resolve through os.Root's fd.
func TestAccessFdRelativeSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("data"), 0o644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link.txt")))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	assert.NoError(t, sb.Access("link.txt", dir, 0x04))
}

// TestAccessFdRelativeEscapeBlocked verifies that symlink escapes
// are blocked at the os.Root level for both Stat and OpenFile.
func TestAccessFdRelativeEscapeBlocked(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")))
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	assert.Error(t, sb.Access("escape.txt", dir, 0x04))
}

// TestAccessSocketFallsBackToStat verifies that Unix sockets, which
// cannot be opened with open(2), fall back to Stat + effectiveHasPerm.
func TestAccessSocketFallsBackToStat(t *testing.T) {
	// Unix socket paths have a ~104-byte limit on macOS. Use a short
	// temp directory to avoid EINVAL from bind(2).
	dir, err := os.MkdirTemp("/tmp", "sock")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, "s.sock")

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	require.NoError(t, err)
	defer syscall.Close(fd)
	require.NoError(t, syscall.Bind(fd, &syscall.SockaddrUnix{Name: sockPath}))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	// Socket with default permissions should pass read check via the
	// Stat fallback path (OpenFile fails on sockets).
	assert.NoError(t, sb.Access("s.sock", dir, 0x04))
}

// TestAccessFIFONonBlocking verifies that O_NONBLOCK prevents blocking
// on a FIFO with no writer. Core of the TOCTOU fix: even if an attacker
// swaps a regular file for a FIFO, the open returns immediately.
func TestAccessFIFONonBlocking(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- sb.Access("fifo", dir, 0x04) }()
	select {
	case err := <-done:
		assert.NoError(t, err)
		// With O_NONBLOCK, this should complete in well under 100ms.
		assert.Less(t, time.Since(start), 500*time.Millisecond,
			"FIFO access took too long — O_NONBLOCK may not be working")
	case <-time.After(2 * time.Second):
		t.Fatal("Access blocked on FIFO — O_NONBLOCK not effective")
	}
}

func TestFIFOReadSeesEmptyWriterEOF(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()
	reader, err := sb.Open("fifo", dir, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer reader.Close()

	writer, err := os.OpenFile(filepath.Join(dir, "fifo"), os.O_WRONLY, 0)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	type readResult struct {
		n   int
		err error
	}
	result := make(chan readResult, 1)
	go func() {
		var buf [1]byte
		n, readErr := reader.Read(buf[:])
		result <- readResult{n: n, err: readErr}
	}()
	select {
	case got := <-result:
		assert.Zero(t, got.n)
		assert.ErrorIs(t, got.err, io.EOF)
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO read did not observe the empty writer's EOF")
	}
}

func TestOpenReadNonblockingFlag(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "regular"), []byte("data"), 0o644))
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	regular, err := sb.Open("regular", dir, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer regular.Close()
	regularFile, ok := regular.(*os.File)
	require.True(t, ok)
	flags, err := unix.FcntlInt(regularFile.Fd(), unix.F_GETFL, 0)
	require.NoError(t, err)
	assert.Zero(t, flags&unix.O_NONBLOCK)

	fifo, err := sb.Open("fifo", dir, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer fifo.Close()
	nonblocking, ok := fifo.(*nonblockingFIFO)
	require.True(t, ok)
	flags, err = unix.FcntlInt(nonblocking.file.Fd(), unix.F_GETFL, 0)
	require.NoError(t, err)
	assert.NotZero(t, flags&unix.O_NONBLOCK)
}

func TestOpenRegularRejectsFIFO(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	handle, err := sb.OpenRegular("fifo", dir)
	require.Nil(t, handle)
	assert.ErrorIs(t, err, writeopen.ErrNotRegularFile)
}

func TestOpenRegularRejectsRacedInFIFONonBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	replacement := filepath.Join(dir, "replacement")
	require.NoError(t, os.WriteFile(path, []byte("regular"), 0o644))
	require.NoError(t, syscall.Mkfifo(replacement, 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	done := make(chan error, 1)
	go func() {
		var swapErr error
		handle, openErr := sb.openRegular("target", dir, func() {
			swapErr = os.Rename(replacement, path)
		})
		if handle != nil {
			_ = handle.Close()
		}
		if swapErr != nil {
			done <- swapErr
			return
		}
		done <- openErr
	}()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, writeopen.ErrNotRegularFile)
	case <-time.After(2 * time.Second):
		t.Fatal("OpenRegular blocked after a regular file was replaced with a FIFO")
	}
}

// TestWriteRegularFileRejectsFIFONoReader verifies the P2 fix directly: a
// FIFO with no reader must be rejected immediately (via O_NONBLOCK) rather
// than blocking the write-open indefinitely.
func TestWriteRegularFileRejectsFIFONoReader(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		done <- func() error {
			_, err := sb.WriteRegularFile(context.Background(), "fifo", dir, []byte("new"), nil)
			return err
		}()
	}()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, writeopen.ErrNotRegularFile)
		assert.Less(t, time.Since(start), 500*time.Millisecond,
			"WriteRegularFile on a FIFO took too long — O_NONBLOCK may not be working")
	case <-time.After(2 * time.Second):
		t.Fatal("WriteRegularFile blocked on a readerless FIFO — O_NONBLOCK not effective")
	}
}

// TestWriteRegularFileRejectsFIFOWithReader verifies that a FIFO is
// rejected via the post-open fstat guard even when a reader is attached (so
// the open itself succeeds and the fstat guard, not ENXIO, is what rejects
// it) — the user-visible outcome must be identical to the no-reader case.
func TestWriteRegularFileRejectsFIFOWithReader(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifoPath, 0o644))

	reader, err := os.OpenFile(fifoPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	_, err = sb.WriteRegularFile(context.Background(), "fifo", dir, []byte("new"), nil)
	assert.ErrorIs(t, err, writeopen.ErrNotRegularFile)
}

// TestWriteRegularFileRejectsRacedInFIFONonBlocking is WriteRegularFile's
// analogue of TestOpenRegularRejectsRacedInFIFONonBlocking above: it proves
// that even if the target is swapped for a FIFO in the narrow window between
// path resolution and the open syscall, the single-fd
// resolve-open-fstat-write sequence still rejects it (via the fstat guard,
// since the FIFO has no reader here either) rather than blocking or writing
// through to whatever the swapped target turns out to be.
func TestWriteRegularFileRejectsRacedInFIFONonBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	replacement := filepath.Join(dir, "replacement")
	require.NoError(t, os.WriteFile(path, []byte("regular"), 0o644))
	require.NoError(t, syscall.Mkfifo(replacement, 0o644))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	// There is no swap-injection hook on the public WriteRegularFile path
	// (unlike openRegular's test-only beforeOpen callback), so this test
	// exercises the realistic sequential race instead: rename the FIFO over
	// the target immediately before calling WriteRegularFile. This does not
	// prove the exact instruction-level TOCTOU window is closed — that
	// guarantee comes from the fstat-on-fd sequencing shared with Truncate,
	// already covered by TestOpenRegularRejectsRacedInFIFONonBlocking — but
	// it does pin the observable, user-facing contract: a FIFO at the
	// resolved path is always rejected, never blocked on and never written
	// through to.
	require.NoError(t, os.Rename(replacement, path))

	done := make(chan error, 1)
	go func() {
		done <- func() error {
			_, err := sb.WriteRegularFile(context.Background(), "target", dir, []byte("new"), nil)
			return err
		}()
	}()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, writeopen.ErrNotRegularFile)
	case <-time.After(2 * time.Second):
		t.Fatal("WriteRegularFile blocked after the target was replaced with a FIFO")
	}
}

func TestOpenRegularRejectsRacedInDifferentRegularFile(t *testing.T) {
	// A regular-file swap reaches the identity check rather than the mode check.
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	replacement := filepath.Join(dir, "replacement")
	require.NoError(t, os.WriteFile(path, []byte("expected"), 0o644))
	require.NoError(t, os.WriteFile(replacement, []byte("substituted"), 0o644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	var swapErr error
	handle, openErr := sb.openRegular("target", dir, func() {
		swapErr = os.Rename(replacement, path)
	})
	if handle != nil {
		_ = handle.Close()
	}
	require.NoError(t, swapErr)
	assert.ErrorContains(t, openErr, "file identity changed while opening")
}

func TestAllowedPathReadOnlyModeRejectsWriteOpenThroughSymlink(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o755))
	target := filepath.Join(child, "child.txt")
	require.NoError(t, os.WriteFile(target, []byte("child"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join("child", "child.txt"), filepath.Join(parent, "link.txt")))

	sb, _, err := New([]string{parent + ":rw", child + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	f, err := sb.Open("link.txt", parent, os.O_WRONLY|os.O_TRUNC, 0)
	assert.Nil(t, f)
	assert.ErrorIs(t, err, os.ErrPermission)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "child", string(got))
}

func TestAllowedPathReadOnlyModeRejectsTruncateThroughSymlink(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.Mkdir(child, 0o755))
	target := filepath.Join(child, "child.txt")
	require.NoError(t, os.WriteFile(target, []byte("child"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join("child", "child.txt"), filepath.Join(parent, "link.txt")))

	sb, _, err := New([]string{parent + ":rw", child + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Truncate("link.txt", parent, 0, false)
	assert.ErrorIs(t, err, os.ErrPermission)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "child", string(got))
}

func TestAllowedPathReadOnlyModeRejectsWriteThroughSymlinkedRoot(t *testing.T) {
	realParent := t.TempDir()
	realRoot := filepath.Join(realParent, "real")
	require.NoError(t, os.Mkdir(realRoot, 0o755))
	target := filepath.Join(realRoot, "file.txt")
	require.NoError(t, os.WriteFile(target, []byte("real"), 0o644))

	linkParent := t.TempDir()
	linkRoot := filepath.Join(linkParent, "link")
	require.NoError(t, os.Symlink(realRoot, linkRoot))

	sb, _, err := New([]string{linkRoot + ":rw", realRoot + ":ro"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	f, err := sb.Open("file.txt", linkRoot, os.O_WRONLY|os.O_TRUNC, 0)
	assert.Nil(t, f)
	assert.ErrorIs(t, err, os.ErrPermission)

	err = sb.Truncate("file.txt", linkRoot, 0, false)
	assert.ErrorIs(t, err, os.ErrPermission)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "real", string(got))
}

func TestSandboxRejectsWriteOpenThroughSymlinkInsideReadWriteRoot(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("target"), 0o644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link.txt")))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	f, err := sb.Open("link.txt", dir, os.O_WRONLY|os.O_TRUNC, 0)
	assert.Nil(t, f)
	assert.Error(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "target", string(got))
}

func TestSandboxRejectsTruncateThroughSymlinkInsideReadWriteRoot(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("target"), 0o644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link.txt")))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	err = sb.Truncate("link.txt", dir, 0, false)
	assert.Error(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "target", string(got))
}

func TestSandboxSymlinkWriteTargetReportsActionableError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.log")
	require.NoError(t, os.WriteFile(target, []byte("target"), 0o644))
	require.NoError(t, os.Symlink("target.log", filepath.Join(dir, "app.log")))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	sizeBefore, truncated, err := sb.TruncateToZeroIfAtLeast("app.log", dir, 0, true)

	assert.Zero(t, sizeBefore)
	assert.False(t, truncated)
	require.Error(t, err)
	assert.Equal(t, "symlinks are not supported as write targets", PortableErrMsg(err))
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "target", string(got))
}

func TestSandboxSymlinkWriteDirectoryComponentReportsActionableError(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	require.NoError(t, os.Mkdir(realDir, 0o755))
	target := filepath.Join(realDir, "app.log")
	require.NoError(t, os.WriteFile(target, []byte("target"), 0o644))
	require.NoError(t, os.Symlink("real", filepath.Join(dir, "link")))

	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	defer sb.Close()
	sb.SetWritable()

	sizeBefore, truncated, err := sb.TruncateToZeroIfAtLeast(filepath.Join("link", "app.log"), dir, 0, true)

	assert.Zero(t, sizeBefore)
	assert.False(t, truncated)
	require.Error(t, err)
	assert.Equal(t, "symlinks are not supported as write targets", PortableErrMsg(err))
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "target", string(got))
}

// --- Cross-root symlink tests ---

// TestCrossRootSymlinkOpen verifies that a symlink in one allowed root
// pointing to a file in another allowed root can be opened.
func TestCrossRootSymlinkOpen(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(dir1, "file.txt"), filepath.Join(dir2, "link.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	f, err := sb.Open("link.txt", dir2, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	assert.Equal(t, "hello", string(buf[:n]))
}

// TestCrossRootSymlinkStat verifies that Stat follows a cross-root symlink.
func TestCrossRootSymlinkStat(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(dir1, "file.txt"), filepath.Join(dir2, "link.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	info, err := sb.Stat("link.txt", dir2)
	require.NoError(t, err)
	assert.Equal(t, int64(5), info.Size())
	assert.Zero(t, info.Mode()&os.ModeSymlink, "Stat should follow the symlink")
}

// TestCrossRootSymlinkAccess verifies that Access works through a cross-root symlink.
func TestCrossRootSymlinkAccess(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(dir1, "file.txt"), filepath.Join(dir2, "link.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("link.txt", dir2, modeRead))
}

// TestCrossRootSymlinkRelativeTarget verifies cross-root resolution with
// a relative symlink target (e.g. ../dir1/file.txt).
func TestCrossRootSymlinkRelativeTarget(t *testing.T) {
	parent := t.TempDir()
	dir1 := filepath.Join(parent, "dir1")
	dir2 := filepath.Join(parent, "dir2")
	require.NoError(t, os.MkdirAll(dir1, 0o755))
	require.NoError(t, os.MkdirAll(dir2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("data"), 0o644))
	require.NoError(t, os.Symlink("../dir1/file.txt", filepath.Join(dir2, "link.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	f, err := sb.Open("link.txt", dir2, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	assert.Equal(t, "data", string(buf[:n]))
}

// TestCrossRootSymlinkIntermediateDir verifies that an intermediate directory
// symlink crossing roots is resolved correctly.
func TestCrossRootSymlinkIntermediateDir(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	subdir := filepath.Join(dir1, "sub")
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("deep"), 0o644))
	// dir2/link -> dir1/sub (directory symlink)
	require.NoError(t, os.Symlink(subdir, filepath.Join(dir2, "link")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	f, err := sb.Open(filepath.Join("link", "file.txt"), dir2, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	assert.Equal(t, "deep", string(buf[:n]))
}

// TestCrossRootSymlinkOutsideAllRoots verifies that a symlink pointing
// outside all allowed roots is still blocked.
func TestCrossRootSymlinkOutsideAllRoots(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir1, "escape.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.Open("escape.txt", dir1, os.O_RDONLY, 0)
	assert.Error(t, err)
}

// TestCrossRootSymlinkMissingTarget verifies that a cross-root symlink
// pointing to a non-existent file returns ENOENT, not the escape error.
func TestCrossRootSymlinkMissingTarget(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	// link points into dir1, but the target file doesn't exist.
	require.NoError(t, os.Symlink(filepath.Join(dir1, "missing.txt"), filepath.Join(dir2, "link.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.Open("link.txt", dir2, os.O_RDONLY, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory", "should report file not found, not path escape")
}

// TestCrossRootSymlinkLstatPreservesTerminal verifies that Lstat through a
// cross-root intermediate symlink preserves the terminal symlink.
func TestCrossRootSymlinkLstatPreservesTerminal(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	// dir1/sub/leaf.txt -> some_target (a symlink)
	subdir := filepath.Join(dir1, "sub")
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "real.txt"), []byte("data"), 0o644))
	require.NoError(t, os.Symlink("real.txt", filepath.Join(subdir, "leaf.txt")))
	// dir2/bridge -> dir1/sub (cross-root directory symlink)
	require.NoError(t, os.Symlink(subdir, filepath.Join(dir2, "bridge")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	// Lstat should report leaf.txt as a symlink, not the target.
	info, err := sb.Lstat(filepath.Join("bridge", "leaf.txt"), dir2)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "Lstat should report symlink mode for terminal component")
}

// TestCrossRootSymlinkReadlinkPreservesTerminal verifies that Readlink
// through a cross-root intermediate symlink reads the terminal symlink.
func TestCrossRootSymlinkReadlinkPreservesTerminal(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	subdir := filepath.Join(dir1, "sub")
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "real.txt"), []byte("data"), 0o644))
	require.NoError(t, os.Symlink("real.txt", filepath.Join(subdir, "leaf.txt")))
	require.NoError(t, os.Symlink(subdir, filepath.Join(dir2, "bridge")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	target, err := sb.Readlink(filepath.Join("bridge", "leaf.txt"), dir2)
	require.NoError(t, err)
	assert.Equal(t, "real.txt", target)
}

// TestCrossRootSymlinkLoopBlocked verifies that circular symlinks between
// roots are detected and rejected after maxSymlinkHops.
func TestCrossRootSymlinkLoopBlocked(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	// dir1/a.txt -> dir2/b.txt -> dir1/a.txt (circular)
	require.NoError(t, os.Symlink(filepath.Join(dir2, "b.txt"), filepath.Join(dir1, "a.txt")))
	require.NoError(t, os.Symlink(filepath.Join(dir1, "a.txt"), filepath.Join(dir2, "b.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.Open("a.txt", dir1, os.O_RDONLY, 0)
	assert.Error(t, err, "circular cross-root symlinks should be rejected")

	_, err = sb.Stat("a.txt", dir1)
	assert.Error(t, err, "circular cross-root symlinks should be rejected")
}

// TestCrossRootSymlinkChainLimit verifies that a long chain of cross-root
// symlinks exceeding maxSymlinkHops is rejected.
func TestCrossRootSymlinkChainLimit(t *testing.T) {
	dirs := make([]string, maxSymlinkHops+2)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}
	// Last directory has the real file.
	require.NoError(t, os.WriteFile(filepath.Join(dirs[len(dirs)-1], "file.txt"), []byte("end"), 0o644))

	// Each dir[i]/link.txt -> dir[i+1]/link.txt, except the penultimate
	// which points to the real file.
	for i := 0; i < len(dirs)-1; i++ {
		target := filepath.Join(dirs[i+1], "link.txt")
		if i == len(dirs)-2 {
			target = filepath.Join(dirs[i+1], "file.txt")
		}
		require.NoError(t, os.Symlink(target, filepath.Join(dirs[i], "link.txt")))
	}

	sb, _, err := New(dirs)
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.Open("link.txt", dirs[0], os.O_RDONLY, 0)
	assert.Error(t, err, "symlink chain exceeding maxSymlinkHops should be rejected")
}

// TestCrossRootSymlinkSiblingDirs verifies that a symlink in one sibling
// directory pointing into another sibling directory can be read when both
// are in AllowedPaths.
func TestCrossRootSymlinkSiblingDirs(t *testing.T) {
	parent := t.TempDir()
	dir1 := filepath.Join(parent, "dir1")
	dir2 := filepath.Join(parent, "dir2")
	require.NoError(t, os.MkdirAll(dir1, 0o755))
	require.NoError(t, os.MkdirAll(dir2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("abc"), 0o644))
	// dir2/sym.txt -> ../dir1/file.txt
	require.NoError(t, os.Symlink("../dir1/file.txt", filepath.Join(dir2, "sym.txt")))

	sb, _, err := New([]string{dir1, dir2})
	require.NoError(t, err)
	defer sb.Close()

	f, err := sb.Open(filepath.Join(dir2, "sym.txt"), "/", os.O_RDONLY, 0)
	require.NoError(t, err, "cross-root symlink between sibling dirs should be readable")
	defer f.Close()

	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	assert.Equal(t, "abc", string(buf[:n]))

	info, err := sb.Stat(filepath.Join(dir2, "sym.txt"), "/")
	require.NoError(t, err)
	assert.Equal(t, int64(3), info.Size())

	assert.NoError(t, sb.Access(filepath.Join(dir2, "sym.txt"), "/", modeRead))
}

// --- Container /host prefix tests ---

// newContainerSandbox creates a sandbox with a host prefix set,
// simulating a containerized environment where host filesystems
// are mounted under prefix.
func newContainerSandbox(t *testing.T, paths []string, hostPrefix string) *Sandbox {
	t.Helper()
	sb, _, err := New(paths)
	require.NoError(t, err)
	sb.SetHostPrefix(hostPrefix)
	return sb
}

// setupContainerDirs creates a directory structure simulating a container
// where host paths are mounted under a prefix. The symlink target is a
// "host-absolute" path (e.g. /var/log/pods/app.log) that only resolves
// after the host prefix is prepended. The symlink is dangling on the test
// machine, but our code reads it via Readlink and constructs the correct
// path.
func setupContainerDirs(t *testing.T) (hostPrefix, pods, containers string) {
	t.Helper()
	root := t.TempDir()
	hostPrefix = root
	pods = filepath.Join(root, "var", "log", "pods")
	containers = filepath.Join(root, "var", "log", "containers")
	require.NoError(t, os.MkdirAll(pods, 0o755))
	require.NoError(t, os.MkdirAll(containers, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pods, "app.log"), []byte("log line"), 0o644))
	// Symlink points to "/var/log/pods/app.log" — a host-absolute path.
	// This is dangling on the test machine, but after our code prepends
	// hostPrefix (root), it becomes root/var/log/pods/app.log which exists.
	require.NoError(t, os.Symlink("/var/log/pods/app.log", filepath.Join(containers, "app.log")))
	return hostPrefix, pods, containers
}

// TestContainerSymlinkHostPrefixOpen verifies that a symlink using a
// host-absolute path (without the mount prefix) can be opened when
// containerized.
func TestContainerSymlinkHostPrefixOpen(t *testing.T) {
	hostPrefix, pods, containers := setupContainerDirs(t)

	sb := newContainerSandbox(t, []string{pods, containers}, hostPrefix)
	defer sb.Close()

	f, err := sb.Open("app.log", containers, os.O_RDONLY, 0)
	require.NoError(t, err, "container symlink with host prefix should be readable")
	defer f.Close()

	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	assert.Equal(t, "log line", string(buf[:n]))
}

// TestContainerSymlinkHostPrefixStat verifies Stat through a container
// symlink with host-absolute target.
func TestContainerSymlinkHostPrefixStat(t *testing.T) {
	hostPrefix, pods, containers := setupContainerDirs(t)

	sb := newContainerSandbox(t, []string{pods, containers}, hostPrefix)
	defer sb.Close()

	info, err := sb.Stat("app.log", containers)
	require.NoError(t, err)
	assert.Equal(t, int64(8), info.Size()) // "log line" = 8 bytes
}

// TestContainerSymlinkHostPrefixAccess verifies Access through a container
// symlink with host-absolute target.
func TestContainerSymlinkHostPrefixAccess(t *testing.T) {
	hostPrefix, pods, containers := setupContainerDirs(t)

	sb := newContainerSandbox(t, []string{pods, containers}, hostPrefix)
	defer sb.Close()

	assert.NoError(t, sb.Access("app.log", containers, modeRead))
}

// TestContainerSymlinkHostPrefixNotAppliedWhenNotContainerized verifies
// that the host prefix is NOT prepended when not in a container.
func TestContainerSymlinkHostPrefixNotAppliedWhenNotContainerized(t *testing.T) {
	// Use the same setup but don't enable containerized mode.
	_, pods, containers := setupContainerDirs(t)

	sb, _, err := New([]string{pods, containers})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.Open("app.log", containers, os.O_RDONLY, 0)
	assert.Error(t, err, "without container mode, host-absolute symlinks should fail")
}

// TestContainerSymlinkHostPrefixOutsideAllRoots verifies that a container
// symlink pointing outside all allowed roots is still blocked even with
// the host prefix applied.
func TestContainerSymlinkHostPrefixOutsideAllRoots(t *testing.T) {
	root := t.TempDir()
	hostPrefix := root
	containers := filepath.Join(root, "var", "log", "containers")
	require.NoError(t, os.MkdirAll(containers, 0o755))
	// /etc/secret exists under the host prefix, but is NOT in allowed roots.
	outside := filepath.Join(root, "etc", "secret")
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "passwd"), []byte("secret"), 0o644))
	// Symlink uses host-absolute path to /etc/secret/passwd.
	require.NoError(t, os.Symlink("/etc/secret/passwd", filepath.Join(containers, "escape.log")))

	sb := newContainerSandbox(t, []string{containers}, hostPrefix)
	defer sb.Close()

	_, err := sb.Open("escape.log", containers, os.O_RDONLY, 0)
	assert.Error(t, err, "container symlink to path outside allowed roots should be blocked")
}

// TestContainerSymlinkRelativeTarget verifies that a relative symlink in
// container mode resolves correctly without double-prepending the prefix.
func TestContainerSymlinkRelativeTarget(t *testing.T) {
	root := t.TempDir()
	hostPrefix := root
	pods := filepath.Join(root, "var", "log", "pods")
	containers := filepath.Join(root, "var", "log", "containers")
	require.NoError(t, os.MkdirAll(pods, 0o755))
	require.NoError(t, os.MkdirAll(containers, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pods, "app.log"), []byte("relative"), 0o644))
	// Relative symlink — target already resolves within the host prefix.
	require.NoError(t, os.Symlink("../pods/app.log", filepath.Join(containers, "app.log")))

	sb := newContainerSandbox(t, []string{pods, containers}, hostPrefix)
	defer sb.Close()

	f, err := sb.Open("app.log", containers, os.O_RDONLY, 0)
	require.NoError(t, err, "relative symlink in container mode should not double-prepend prefix")
	defer f.Close()

	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	assert.Equal(t, "relative", string(buf[:n]))
}
