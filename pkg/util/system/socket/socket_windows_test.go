// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package socket

import (
	"os"
	"testing"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeout = 500 * time.Millisecond

func TestIsAvailableNotExist(t *testing.T) {
	exists, err := IsAvailable(`\\.\pipe\dd-test-does-not-exist`, testTimeout)
	assert.False(t, exists)
	assert.NoError(t, err)
}

func TestIsAvailableReachable(t *testing.T) {
	pipePath := `\\.\pipe\dd-test-reachable`
	listener, err := winio.ListenPipe(pipePath, nil)
	require.NoError(t, err)
	defer listener.Close()

	// A real named pipe server (e.g. the Docker daemon) is actively accepting
	// connections; without a pending Accept() the server never completes the
	// connect handshake, and the client dial can time out on loaded CI hosts.
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	exists, availErr := IsAvailable(pipePath, testTimeout)
	assert.True(t, exists)
	assert.NoError(t, availErr)
}

// TestIsAvailablePermissionDenied uses a security descriptor that denies the
// current user access to the pipe, so DialPipe fails with access-denied.
func TestIsAvailablePermissionDenied(t *testing.T) {
	pipePath := `\\.\pipe\dd-test-permission-denied`
	// Deny generic access to everyone (World SID).
	sd := "D:(D;;GA;;;WD)"
	listener, err := winio.ListenPipe(pipePath, &winio.PipeConfig{SecurityDescriptor: sd})
	require.NoError(t, err)
	defer listener.Close()

	exists, availErr := IsAvailable(pipePath, testTimeout)
	assert.True(t, exists)
	assert.ErrorIs(t, availErr, os.ErrPermission)
}
