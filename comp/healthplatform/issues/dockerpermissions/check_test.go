// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package dockerpermissions

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shortTempDir returns a short temp dir, since unix socket paths are limited
// to ~104 bytes and t.TempDir() paths derived from test names can exceed that.
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
	// Leave the socket file on disk to simulate a stale socket left behind by
	// a process that stopped listening (connection refused).
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
