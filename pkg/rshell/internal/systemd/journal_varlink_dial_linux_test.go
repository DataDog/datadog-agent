// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package systemd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialPinnedJournalControlIgnoresPathReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "journal.sock")
	original := listenUnixSocket(t, path)

	socket, err := (&Client{}).openJournalControlSocket(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = socket.Close() })

	require.NoError(t, os.Rename(path, filepath.Join(dir, "original.sock")))
	attacker := listenUnixSocket(t, path)

	conn, err := dialPinnedJournalControl(context.Background(), int(socket.Fd()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	require.NoError(t, original.SetDeadline(time.Now().Add(time.Second)))
	accepted, err := original.AcceptUnix()
	require.NoError(t, err)
	require.NoError(t, accepted.Close())

	require.NoError(t, attacker.SetDeadline(time.Now().Add(100*time.Millisecond)))
	_, err = attacker.AcceptUnix()
	require.Error(t, err)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout())
}

func listenUnixSocket(t *testing.T, path string) *net.UnixListener {
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	listener.SetUnlinkOnClose(false)
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}
