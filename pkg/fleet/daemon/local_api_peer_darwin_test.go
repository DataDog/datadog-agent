// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerUIDReadsTheConnectingProcess(t *testing.T) {
	// A short directory: macOS caps a socket path at 104 bytes, which t.TempDir can exceed.
	dir, err := os.MkdirTemp("/tmp", "peer")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	listener, err := net.Listen("unix", filepath.Join(dir, "s"))
	require.NoError(t, err)
	defer listener.Close()

	client, err := net.Dial("unix", listener.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	server, err := listener.Accept()
	require.NoError(t, err)
	defer server.Close()

	uid, ok := peerUID(server)
	require.True(t, ok)
	assert.Equal(t, uint32(os.Geteuid()), uid)
}

func TestPeerUIDIsUnknownOverTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	server, err := listener.Accept()
	require.NoError(t, err)
	defer server.Close()

	_, ok := peerUID(server)
	assert.False(t, ok)
}
