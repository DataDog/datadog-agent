// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempSocketPath returns a short path for a test socket. t.TempDir() is not used
// because on macOS its prefix plus a test name easily exceeds the 104-byte limit
// on unix socket paths.
func tempSocketPath(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "dd-inst")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return filepath.Join(dir, "s.sock")
}

// serveStatusSocket binds and serves the status socket at socketPath, going through
// the same listenStatusSocket the daemon uses so the permission recipe is what is
// under test.
func serveStatusSocket(t *testing.T, daemon statusProvider, socketPath string) {
	t.Helper()

	listener, err := listenStatusSocket(socketPath)
	require.NoError(t, err)
	api := newStatusAPI(daemon, listener)
	require.NoError(t, api.Start(context.Background()))
	t.Cleanup(func() { _ = api.Stop(context.Background()) })
}

func startTestStatusAPI(t *testing.T, response StatusAPIResponse) string {
	t.Helper()

	socketPath := tempSocketPath(t)
	serveStatusSocket(t, &testStatusProvider{response: response}, socketPath)

	return socketPath
}

// The whole point of this second listener is that the Agent user can reach it,
// unlike the daemon's 0700 local API. If the mode regresses the Agent silently
// starts reporting the installer as unreachable, so pin it.
func TestStatusAPISocketPermissions(t *testing.T) {
	socketPath := startTestStatusAPI(t, StatusAPIResponse{})

	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSocket, "expected a unix socket")
	assert.Equal(t, os.FileMode(0720), info.Mode().Perm())
}

// The socket lives in a directory dd-agent can write to, so an unprivileged
// process can plant a regular file at the path. Refusing to start on it would hand
// that process a way to stop the daemon, so the file is replaced instead.
func TestStatusAPIReplacesPlantedFile(t *testing.T) {
	socketPath := tempSocketPath(t)
	require.NoError(t, os.WriteFile(socketPath, []byte("not a socket"), 0600))

	serveStatusSocket(t, &testStatusProvider{}, socketPath)

	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSocket, "expected the planted file to be replaced by a socket")
}

// The mode comes from the umask at bind time rather than a chmod afterwards, so it
// has to hold whatever the process umask happens to be.
func TestStatusAPISocketModeIgnoresProcessUmask(t *testing.T) {
	// Before the swap: the socket's own directory has to stay usable.
	socketPath := tempSocketPath(t)

	umaskMu.Lock()
	previous := syscall.Umask(0777)
	umaskMu.Unlock()
	t.Cleanup(func() {
		umaskMu.Lock()
		syscall.Umask(previous)
		umaskMu.Unlock()
	})

	serveStatusSocket(t, &testStatusProvider{}, socketPath)

	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0720), info.Mode().Perm())
}

// A stale socket from a previous daemon run must be replaced, not rejected.
func TestStatusAPIReplacesStaleSocket(t *testing.T) {
	socketPath := tempSocketPath(t)
	stale, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	// Keep the socket file behind so it looks like a crashed daemon's leftover.
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())

	serveStatusSocket(t, &testStatusProvider{}, socketPath)
}
