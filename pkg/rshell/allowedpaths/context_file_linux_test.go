// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package allowedpaths

// These tests verify that WithContextClose correctly guarantees Close() is
// called when the context is cancelled, and that — for file types backed by
// Go's network poller (pipes, sockets, char devices) — a goroutine blocked in
// Read() is unblocked when Close() fires. Regular files never block so their
// section only checks that Read returns before Close is called.
//
// Each test opens or creates the file type under test directly via os / syscall
// (bypassing the sandbox) since WithContextClose is the unit under test here.

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// blockReadTimeout is how long we wait for a blocked Read to unblock after
// the context is cancelled before declaring the test a failure.
const blockReadTimeout = 3 * time.Second

// readResult carries the outcome of a single Read call.
type readResult struct {
	n   int
	err error
}

// launchRead starts a goroutine that calls Read once on f and sends the
// result to the returned channel. The goroutine runs until Read returns.
func launchRead(f io.Reader) <-chan readResult {
	ch := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 128)
		n, err := f.Read(buf)
		ch <- readResult{n, err}
	}()
	return ch
}

// awaitRead waits up to timeout for a result on ch.
// Returns (result, true) if one arrives, or (zero, false) on timeout.
func awaitRead(ch <-chan readResult, timeout time.Duration) (readResult, bool) {
	select {
	case r := <-ch:
		return r, true
	case <-time.After(timeout):
		return readResult{}, false
	}
}

// ---------------------------------------------------------------------------
// Non-blocking file types — Read returns immediately; tests verify no hang.
// ---------------------------------------------------------------------------

// TestWithContextClose_Linux_RegularFile verifies that Read on a regular file
// (which always returns immediately) works correctly with WithContextClose.
// Regular files are not registered with the epoll poller, but they never
// block so there is no risk of a leaked goroutine.
func TestWithContextClose_Linux_RegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello world"), 0600))

	f, err := os.Open(path)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on regular file hung — unexpected for a non-blocking fd")
	}
	t.Logf("regular file: n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_DevNull verifies /dev/null.
// Read always returns (0, io.EOF) immediately.
func TestWithContextClose_Linux_DevNull(t *testing.T) {
	f, err := os.Open("/dev/null")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on /dev/null hung — unexpected")
	}
	t.Logf("/dev/null: n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_DevZero verifies /dev/zero.
// Read always returns immediately with a buffer full of zero bytes.
func TestWithContextClose_Linux_DevZero(t *testing.T) {
	f, err := os.Open("/dev/zero")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on /dev/zero hung — unexpected (should return zeros immediately)")
	}
	t.Logf("/dev/zero: n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_DevUrandom verifies /dev/urandom.
// Read always returns immediately with random bytes.
func TestWithContextClose_Linux_DevUrandom(t *testing.T) {
	f, err := os.Open("/dev/urandom")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on /dev/urandom hung — unexpected")
	}
	t.Logf("/dev/urandom: n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_SymlinkToFile verifies that reading through a
// symbolic link (which the OS resolves to the target) is identical to reading
// the target directly. Read returns immediately.
func TestWithContextClose_Linux_SymlinkToFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("symlink target data"), 0600))
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink(target, link))

	f, err := os.Open(link) // follows the symlink
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on symlink-to-file hung — unexpected")
	}
	t.Logf("symlink→file: n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_Directory verifies behavior when reading a
// directory fd directly. On Linux, read(2) on a directory fd returns EISDIR
// immediately — it never blocks.
func TestWithContextClose_Linux_Directory(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Open(dir)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on directory hung — expected immediate EISDIR")
	}
	t.Logf("directory: n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_BlockDevice verifies a block device. Block
// devices return data in fixed-size chunks and never block on read (they
// return an error if there is no underlying storage). We try several
// candidates and skip if none is accessible.
func TestWithContextClose_Linux_BlockDevice(t *testing.T) {
	candidates := []string{"/dev/loop0", "/dev/sda", "/dev/vda", "/dev/xvda", "/dev/nvme0n1"}
	var f *os.File
	var chosen string
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeDevice == 0 {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			continue // likely permission denied
		}
		f = file
		chosen = path
		break
	}
	if f == nil {
		t.Skip("no readable block device found — skipping block device test")
	}
	t.Logf("using block device: %s", chosen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatalf("Read on block device %s hung — unexpected", chosen)
	}
	t.Logf("block device %s: n=%d err=%v", chosen, r.n, r.err)
}

// ---------------------------------------------------------------------------
// Blocking file types — Read blocks until data arrives or the fd is closed.
// Each test cancels the context after giving Read time to enter its blocked
// state, then verifies it unblocks within blockReadTimeout.
// ---------------------------------------------------------------------------

// TestWithContextClose_Linux_FIFO verifies that a Read blocked on a named
// pipe (FIFO) with no writer is unblocked when the context is cancelled.
//
// The FIFO is opened O_RDWR so that open(2) itself does not block (the
// kernel sees the process holding both ends). With the pipe buffer empty,
// Read blocks waiting for data. Go registers FIFO fds with the epoll poller,
// so Close() calls evict(), waking the blocked goroutine.
func TestWithContextClose_Linux_FIFO(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "test.fifo")
	require.NoError(t, syscall.Mkfifo(fifoPath, 0600))

	// O_RDWR avoids blocking on open() — we hold both read and write ends.
	f, err := os.OpenFile(fifoPath, os.O_RDWR, 0)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	// Allow the goroutine to enter the blocked Read.
	time.Sleep(50 * time.Millisecond)
	cancel()

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on FIFO did not unblock after context cancellation")
	}
	t.Logf("FIFO (blocked): n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_UnixSocket verifies that a Read blocked on a
// Unix-domain socket is unblocked when the context is cancelled.
//
// net.Pipe() creates a connected socketpair; c2 never sends anything, so
// Read on c1 blocks. When Close(c1) fires via context cancel, Go's network
// poller wakes the blocked Read.
func TestWithContextClose_Linux_UnixSocket(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// net.Conn satisfies io.ReadWriteCloser.
	wrapped := WithContextClose(ctx, c1)
	ch := launchRead(wrapped)

	time.Sleep(50 * time.Millisecond)
	cancel()

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on Unix socket did not unblock after context cancellation")
	}
	t.Logf("Unix socket (blocked): n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_DevRandom verifies /dev/random.
// On Linux >= 5.6 /dev/random behaves like /dev/urandom and never blocks.
// On older kernels it may block when entropy is exhausted. In either case,
// context cancellation must unblock the Read.
func TestWithContextClose_Linux_DevRandom(t *testing.T) {
	f, err := os.Open("/dev/random")
	if err != nil {
		t.Skipf("/dev/random not available: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	// Give the read time to potentially block (on old kernels with low entropy).
	time.Sleep(50 * time.Millisecond)
	cancel()

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on /dev/random did not unblock after context cancellation")
	}
	t.Logf("/dev/random (may have blocked): n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_TTY verifies /dev/tty (the controlling terminal).
// Read on /dev/tty blocks waiting for a keystroke. Context cancellation must
// unblock it. The test is skipped if /dev/tty is unavailable (common in CI).
func TestWithContextClose_Linux_TTY(t *testing.T) {
	f, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		t.Skipf("/dev/tty not available: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, f)
	ch := launchRead(wrapped)

	time.Sleep(50 * time.Millisecond)
	cancel()

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on /dev/tty did not unblock after context cancellation")
	}
	t.Logf("/dev/tty (blocked): n=%d err=%v", r.n, r.err)
}

// TestWithContextClose_Linux_UnixSocketFile verifies a filesystem-visible
// Unix domain socket — the kind found under /run or /tmp. We create a
// listening socket, connect a client, then read from the accepted server
// connection with no data sent. Context cancellation must unblock the Read.
func TestWithContextClose_Linux_UnixSocketFile(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "socktest")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	sockPath := filepath.Join(dir, "t.sock")
	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	defer ln.Close()

	// Connect a client so Accept returns a server-side conn.
	clientDone := make(chan net.Conn, 1)
	go func() {
		c, err := net.Dial("unix", sockPath)
		if err != nil {
			clientDone <- nil
			return
		}
		clientDone <- c
	}()

	serverConn, err := ln.Accept()
	require.NoError(t, err)

	client := <-clientDone
	require.NotNil(t, client)
	defer client.Close()

	// server-side conn: client sends nothing, so Read blocks.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wrapped := WithContextClose(ctx, serverConn)
	ch := launchRead(wrapped)

	time.Sleep(50 * time.Millisecond)
	cancel()

	r, ok := awaitRead(ch, blockReadTimeout)
	if !ok {
		t.Fatal("Read on Unix socket file did not unblock after context cancellation")
	}
	t.Logf("Unix socket file (blocked): n=%d err=%v", r.n, r.err)
}
