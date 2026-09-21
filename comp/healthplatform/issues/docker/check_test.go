// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package docker

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortTempDir returns a short temp dir, since t.TempDir() can exceed the ~104-byte unix socket path limit.
func shortTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "dd-sock")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestClassifySockets(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks")
	}

	reachablePath := filepath.Join(shortTempDir(t), "reachable.sock")
	reachableListener, err := net.Listen("unix", reachablePath)
	require.NoError(t, err)
	defer reachableListener.Close()

	permissionPath := filepath.Join(shortTempDir(t), "permission.sock")
	permissionListener, err := net.Listen("unix", permissionPath)
	require.NoError(t, err)
	defer permissionListener.Close()
	require.NoError(t, os.Chmod(permissionPath, 0000))

	unavailablePath := filepath.Join(shortTempDir(t), "unavailable.sock")
	unavailableListener, err := net.Listen("unix", unavailablePath)
	require.NoError(t, err)
	// Leave the socket file on disk to simulate a stale socket (connection refused).
	unavailableListener.(*net.UnixListener).SetUnlinkOnClose(false)
	unavailableListener.Close()

	missingPath := filepath.Join(shortTempDir(t), "missing.sock")

	permissionSockets, unavailableSockets := classifySockets([]string{
		reachablePath, permissionPath, unavailablePath, missingPath,
	})

	assert.Equal(t, []string{permissionPath}, permissionSockets)
	assert.Equal(t, []string{unavailablePath}, unavailableSockets)
}

func TestClassifySockets_AllReachable(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "s.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	permissionSockets, unavailableSockets := classifySockets([]string{socketPath})

	assert.Empty(t, permissionSockets)
	assert.Empty(t, unavailableSockets)
}

func TestMatchingDefaultSocketPath(t *testing.T) {
	socketPaths := []string{"/var/run/docker.sock", "/host/var/run/docker.sock"}

	path, ok := matchingDefaultSocketPath("unix:///var/run/docker.sock", socketPaths)
	assert.True(t, ok, "must match the value detectDocker() auto-sets for the default socket")
	assert.Equal(t, "/var/run/docker.sock", path)

	path, ok = matchingDefaultSocketPath("unix:///host/var/run/docker.sock", socketPaths)
	assert.True(t, ok, "must match the value detectDocker() auto-sets for the host-mounted default socket")
	assert.Equal(t, "/host/var/run/docker.sock", path)

	_, ok = matchingDefaultSocketPath("tcp://remote-docker:2375", socketPaths)
	assert.False(t, ok, "a genuinely custom/remote endpoint must not be mistaken for the auto-set default")

	_, ok = matchingDefaultSocketPath("unix:///some/other/docker.sock", socketPaths)
	assert.False(t, ok, "a custom local socket path must not be mistaken for the auto-set default")
}
