// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package installerimpl

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveStatus serves body on a temporary unix socket and returns its path.
//
// The socket is created under os.MkdirTemp rather than t.TempDir() because on macOS
// that prefix plus a test name easily exceeds the 104-byte limit on socket paths.
// The daemon's own listener is tested in comp/updater/statusapi/impl; what is under
// test here is that the client can dial it and read what it wrote.
func serveStatus(t *testing.T, status int, body string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "dd-inst")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socketPath := filepath.Join(dir, "s.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return socketPath
}

func TestStatusClient(t *testing.T) {
	socketPath := serveStatus(t, http.StatusOK, `{"installer_version":"7.76.0","available_disk_space":12884901888}`)

	status, err := newStatusClient(socketPath).Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "7.76.0", status.InstallerVersion)
	require.NotNil(t, status.AvailableDiskSpace)
	assert.Equal(t, uint64(12884901888), *status.AvailableDiskSpace)
}

// An absent available_disk_space must stay nil rather than become 0, which would
// read as a full disk.
func TestStatusClientUnknownDiskSpace(t *testing.T) {
	socketPath := serveStatus(t, http.StatusOK, `{"installer_version":"7.76.0"}`)

	status, err := newStatusClient(socketPath).Status(context.Background())
	require.NoError(t, err)
	assert.Nil(t, status.AvailableDiskSpace)
}

func TestStatusClientErrors(t *testing.T) {
	tests := []struct {
		name       string
		socketPath func(*testing.T) string
	}{
		{
			name:       "no daemon listening",
			socketPath: func(t *testing.T) string { return filepath.Join(t.TempDir(), "absent.sock") },
		},
		{
			name:       "error response",
			socketPath: func(t *testing.T) string { return serveStatus(t, http.StatusInternalServerError, "") },
		},
		{
			name:       "malformed body",
			socketPath: func(t *testing.T) string { return serveStatus(t, http.StatusOK, "not json") },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newStatusClient(test.socketPath(t)).Status(context.Background())
			assert.Error(t, err)
		})
	}
}
