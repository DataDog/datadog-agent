// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package socket

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeout = 500 * time.Millisecond

// shortTempDir returns a short temp dir, since unix socket paths are limited
// to ~104 bytes and t.TempDir() paths derived from test names can exceed that.
func shortTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "dd-sock")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestIsAvailableNotExist(t *testing.T) {
	exists, err := IsAvailable(filepath.Join(shortTempDir(t), "s.sock"), testTimeout)
	assert.False(t, exists)
	assert.NoError(t, err)
}

func TestIsAvailableReachable(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "s.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	exists, availErr := IsAvailable(socketPath, testTimeout)
	assert.True(t, exists)
	assert.NoError(t, availErr)
}

// TestIsAvailableConnectionRefused mirrors a stale socket file left behind by
// a process that stopped listening: it should still report reachable (nil error).
func TestIsAvailableConnectionRefused(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "s.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	// Leave the socket file on disk to simulate the stale-socket scenario.
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	listener.Close()

	exists, availErr := IsAvailable(socketPath, testTimeout)
	assert.True(t, exists)
	assert.NoError(t, availErr)
}

func TestIsAvailablePermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permission checks")
	}

	socketPath := filepath.Join(shortTempDir(t), "s.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	require.NoError(t, os.Chmod(socketPath, 0000))

	exists, availErr := IsAvailable(socketPath, testTimeout)
	assert.True(t, exists)
	assert.ErrorIs(t, availErr, os.ErrPermission)
}
