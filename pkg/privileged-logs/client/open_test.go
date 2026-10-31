// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package client

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenPrivilegedConnectionReset(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "s.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()
	go func() {
		// Closing with unread request bytes resets the connection.
		if conn, err := listener.Accept(); err == nil {
			_, _ = conn.Read(make([]byte, 1))
			conn.Close()
		}
	}()

	file, err := OpenPrivileged(socketPath, "/var/log/app.log")
	assert.Nil(t, file)
	assert.Error(t, err)
}
