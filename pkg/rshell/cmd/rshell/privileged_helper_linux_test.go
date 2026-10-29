// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOptionalPolicyMissing(t *testing.T) {
	credential, err := loadOptionalPolicy(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	require.Nil(t, credential)
}

func TestLoadOptionalPolicyEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	credential, err := loadOptionalPolicy(path)
	require.NoError(t, err)
	require.Nil(t, credential)
}

// shortTempDir returns an owner-only temporary directory with a path short
// enough for a Unix socket (about 100 bytes).
func shortTempDir(t *testing.T) string {
	t.Helper()
	// createListener requires every ancestor to be owned by root or the caller.
	// An unprivileged Bazel linux-sandbox maps only the caller's uid, so "/"
	// shows up as the overflow uid and no socket path can pass.
	var root syscall.Stat_t
	require.NoError(t, syscall.Stat("/", &root))
	if root.Uid != 0 && int(root.Uid) != os.Geteuid() {
		t.Skipf("/ is owned by uid %d in this user namespace", root.Uid)
	}
	dir, err := os.MkdirTemp("", "rsh")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	require.NoError(t, os.Chmod(dir, 0o755))
	return dir
}

func TestCreateListener(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "h.sock")

	listener, lock, err := createListener(path, os.Getuid(), []int{os.Getgid()})
	require.NoError(t, err)
	t.Cleanup(func() { lock.Close() })

	var stat syscall.Stat_t
	require.NoError(t, syscall.Lstat(path, &stat))
	require.Equal(t, uint32(syscall.S_IFSOCK), stat.Mode&syscall.S_IFMT)
	require.Equal(t, uint32(0o600), stat.Mode&0o777, "only the owner may connect")
	require.Equal(t, uint32(os.Getuid()), stat.Uid)

	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	// After the privilege drop the helper cannot unlink the socket, so it
	// does not try: the supervisor removes it.
	require.NoError(t, listener.Close())
	_, err = os.Lstat(path)
	require.NoError(t, err)
}

func TestCreateListenerReplacesStaleSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "h.sock")
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	stale.SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())

	listener, lock, err := createListener(path, os.Getuid(), []int{os.Getgid()})
	require.NoError(t, err)
	t.Cleanup(func() { lock.Close() })
	require.NoError(t, listener.Close())
}

func TestCreateListenerRefusesServedSocket(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "h.sock")
	served, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	t.Cleanup(func() { served.Close() })

	_, _, err = createListener(path, os.Getuid(), []int{os.Getgid()})
	require.ErrorContains(t, err, "already being served")
	conn, err := net.Dial("unix", path)
	require.NoError(t, err, "the served socket must stay reachable")
	require.NoError(t, conn.Close())
}

func TestCreateListenerRejectsUnsafeTargets(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		_, _, err := createListener("h.sock", os.Getuid(), []int{os.Getgid()})
		require.ErrorContains(t, err, "not absolute")
	})
	t.Run("existing non-socket file", func(t *testing.T) {
		path := filepath.Join(shortTempDir(t), "h.sock")
		require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
		_, _, err := createListener(path, os.Getuid(), []int{os.Getgid()})
		require.ErrorContains(t, err, "is not a socket")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "keep", string(content), "a non-socket file must not be removed")
	})
	t.Run("socket directory writable by others", func(t *testing.T) {
		dir := shortTempDir(t)
		require.NoError(t, os.Chmod(dir, 0o777))
		_, _, err := createListener(filepath.Join(dir, "h.sock"), os.Getuid(), []int{os.Getgid()})
		require.ErrorContains(t, err, "writable by group or others")
	})
	t.Run("sticky socket directory", func(t *testing.T) {
		// --user owns the socket and could replace it in a sticky directory.
		dir := shortTempDir(t)
		require.NoError(t, os.Chmod(dir, 0o777|os.ModeSticky))
		_, _, err := createListener(filepath.Join(dir, "h.sock"), os.Getuid(), []int{os.Getgid()})
		require.ErrorContains(t, err, "writable by group or others")
	})
	t.Run("ancestor writable by others", func(t *testing.T) {
		ancestor := shortTempDir(t)
		dir := filepath.Join(ancestor, "d")
		require.NoError(t, os.Mkdir(dir, 0o755))
		require.NoError(t, os.Chmod(ancestor, 0o777))
		_, _, err := createListener(filepath.Join(dir, "h.sock"), os.Getuid(), []int{os.Getgid()})
		require.ErrorContains(t, err, ancestor+" is writable by group or others")
	})
	t.Run("path is not clean", func(t *testing.T) {
		// The kernel resolves ".." after following link, so a check on the
		// cleaned path would examine the wrong directory.
		parent := shortTempDir(t)
		link := filepath.Join(parent, "link")
		require.NoError(t, os.Symlink(filepath.Join(shortTempDir(t), "sub"), link))
		_, _, err := createListener(link+"/../h.sock", os.Getuid(), []int{os.Getgid()})
		require.ErrorContains(t, err, "is not clean")
	})
	t.Run("directory is a symlink", func(t *testing.T) {
		parent := shortTempDir(t)
		link := filepath.Join(parent, "link")
		require.NoError(t, os.Symlink(shortTempDir(t), link))
		_, _, err := createListener(filepath.Join(link, "h.sock"), os.Getuid(), []int{os.Getgid()})
		require.ErrorContains(t, err, "is not a directory")
	})
}

func TestCreateListenerLocksThePath(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "h.sock")
	listener, lock, err := createListener(path, os.Getuid(), []int{os.Getgid()})
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	_, _, err = createListener(path, os.Getuid(), []int{os.Getgid()})
	require.ErrorContains(t, err, "another helper is serving")

	// The lock outlives the socket: a helper whose socket was removed by
	// someone else still keeps others off the path until it exits.
	require.NoError(t, os.Remove(path))
	_, _, err = createListener(path, os.Getuid(), []int{os.Getgid()})
	require.ErrorContains(t, err, "another helper is serving")

	require.NoError(t, lock.Close())
	listener2, lock2, err := createListener(path, os.Getuid(), []int{os.Getgid()})
	require.NoError(t, err, "the lock is released when its holder closes it")
	require.NoError(t, lock2.Close())
	require.NoError(t, listener2.Close())
}

func TestCreateListenerCleansUpFailedChown(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can chown to any account")
	}
	path := filepath.Join(shortTempDir(t), "h.sock")
	_, _, err := createListener(path, 65534, []int{65534})
	require.ErrorContains(t, err, "chown")
	_, err = os.Lstat(path)
	require.ErrorIs(t, err, os.ErrNotExist, "a socket that was never handed over must not be left behind")

	listener, lock, err := createListener(path, os.Getuid(), []int{os.Getgid()})
	require.NoError(t, err, "a failed start must release the lock")
	require.NoError(t, lock.Close())
	require.NoError(t, listener.Close())
}

func TestCreateListenerRequiresTraversableDirectories(t *testing.T) {
	dir := shortTempDir(t)
	require.NoError(t, os.Chmod(dir, 0o700))
	_, _, err := createListener(filepath.Join(dir, "h.sock"), 65534, []int{65534})
	require.ErrorContains(t, err, "cannot be searched by uid 65534")

	require.NoError(t, os.Chmod(dir, 0o711))
	require.NoError(t, requireTraversable(dir, 65534, []int{65534}))
}

func TestRunPrivilegedHelperSocketFlags(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("checks the flag handling that runs before the root requirement")
	}
	socket := filepath.Join(shortTempDir(t), "h.sock")
	var stderr bytes.Buffer
	code := runPrivilegedHelper(context.Background(), []string{"--socket=" + socket, "--idle-timeout=5s"}, &stderr)
	require.Equal(t, 2, code)
	require.Contains(t, stderr.String(), "--socket requires --idle-timeout=0")

	for _, args := range [][]string{{"--socket=" + socket}, {"--socket=" + socket, "--idle-timeout=0"}} {
		stderr.Reset()
		code = runPrivilegedHelper(context.Background(), args, &stderr)
		require.Equal(t, 1, code, "%v: the flags are accepted", args)
		require.Contains(t, stderr.String(), "requires real uid 0")
	}
	_, err := os.Lstat(socket)
	require.ErrorIs(t, err, os.ErrNotExist, "no socket is created before the helper is ready")
}
