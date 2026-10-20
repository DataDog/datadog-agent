// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package executor

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dirEntries lists the names in dir, to detect leftover staging directories.
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// TestListenRestrictsSocketMode asserts the filesystem mode only; see
// TestListenRejectsOtherUsers for an actual cross-user connection attempt.
func TestListenRestrictsSocketMode(t *testing.T) {
	// A permissive umask must not widen the socket. No test in this package
	// runs in parallel, so the process-wide change is confined to this test.
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(oldUmask) })

	address := testListenAddr(t)
	lis, err := Listen(address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	info, err := os.Lstat(address)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSocket, "expected a socket, got mode %v", info.Mode())
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Equal(t, address, lis.Addr().String())
	assert.Equal(t, []string{filepath.Base(address)}, dirEntries(t, filepath.Dir(address)),
		"the staging directory must be removed")
}

func TestListenServesIntendedClient(t *testing.T) {
	address := testListenAddr(t)
	lis, err := Listen(address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	accepted := make(chan error, 1)
	go func() {
		conn, err := lis.Accept()
		if err != nil {
			accepted <- err
			return
		}
		defer conn.Close()
		_, err = conn.Write([]byte("ok"))
		accepted <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := Dial(ctx, address, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	got, err := io.ReadAll(conn)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(got))
	require.NoError(t, <-accepted)
}

func TestListenReplacesStaleSocket(t *testing.T) {
	address := testListenAddr(t)
	stale, err := net.Listen("unix", address)
	require.NoError(t, err)
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	_, err = os.Lstat(address)
	require.NoError(t, err, "the stale socket file must exist before Listen")

	lis, err := Listen(address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	info, err := os.Lstat(address)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestListenCloseRemovesSocket(t *testing.T) {
	address := testListenAddr(t)
	lis, err := Listen(address)
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	_, err = os.Lstat(address)
	assert.True(t, os.IsNotExist(err), "socket still present after Close: %v", err)
	assert.Empty(t, dirEntries(t, filepath.Dir(address)))
}

func TestListenCloseKeepsReplacementSocket(t *testing.T) {
	address := testListenAddr(t)
	first, err := Listen(address)
	require.NoError(t, err)
	second, err := Listen(address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	require.NoError(t, first.Close())
	_, err = os.Lstat(address)
	assert.NoError(t, err, "closing a replaced listener must not unlink its successor")
}

func TestListenFailsClosedWhenPermissionSetupFails(t *testing.T) {
	chmodErr := errors.New("chmod refused")
	original := chmodSocket
	chmodSocket = func(string, os.FileMode) error { return chmodErr }
	t.Cleanup(func() { chmodSocket = original })

	address := testListenAddr(t)
	lis, err := Listen(address)
	require.ErrorIs(t, err, chmodErr)
	assert.Nil(t, lis)

	_, statErr := os.Lstat(address)
	assert.True(t, os.IsNotExist(statErr), "no socket may be published: %v", statErr)
	assert.Empty(t, dirEntries(t, filepath.Dir(address)), "the staging socket must be cleaned up")
}
