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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daemonstatus "github.com/DataDog/datadog-agent/pkg/fleet/daemon/status"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type testStatusProvider struct {
	response daemonstatus.Response
}

func (t *testStatusProvider) GetStatus() daemonstatus.Response {
	return t.response
}

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

func startTestStatusAPI(t *testing.T, response daemonstatus.Response) string {
	t.Helper()

	socketPath := tempSocketPath(t)
	api, err := newStatusAPI(&testStatusProvider{response: response}, socketPath)
	require.NoError(t, err)
	require.NoError(t, api.Start(context.Background()))
	t.Cleanup(func() { _ = api.Stop(context.Background()) })

	return socketPath
}

func TestStatusAPIRoundTrip(t *testing.T) {
	diskSpace := uint64(12884901888)
	socketPath := startTestStatusAPI(t, daemonstatus.Response{
		InstallerVersion:   version.AgentVersion,
		AvailableDiskSpace: &diskSpace,
	})

	response, err := daemonstatus.NewClient(socketPath).Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, version.AgentVersion, response.InstallerVersion)
	require.NotNil(t, response.AvailableDiskSpace)
	assert.Equal(t, diskSpace, *response.AvailableDiskSpace)
}

// A daemon that could not determine the free space must leave the field unset
// rather than report 0, which would read as a full disk.
func TestStatusAPIOmitsUnknownDiskSpace(t *testing.T) {
	socketPath := startTestStatusAPI(t, daemonstatus.Response{InstallerVersion: "7.76.0"})

	response, err := daemonstatus.NewClient(socketPath).Status(context.Background())
	require.NoError(t, err)
	assert.Nil(t, response.AvailableDiskSpace)
}

// The whole point of this second listener is that the Agent user can reach it,
// unlike the daemon's 0700 local API. If the mode regresses the Agent silently
// starts reporting the installer as unreachable, so pin it.
func TestStatusAPISocketPermissions(t *testing.T) {
	socketPath := startTestStatusAPI(t, daemonstatus.Response{})

	info, err := os.Stat(socketPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSocket, "expected a unix socket")
	assert.Equal(t, os.FileMode(0720), info.Mode().Perm())
}

// A leftover regular file at the socket path must not be silently unlinked.
func TestStatusAPIRefusesNonSocketPath(t *testing.T) {
	socketPath := tempSocketPath(t)
	require.NoError(t, os.WriteFile(socketPath, []byte("not a socket"), 0600))

	_, err := newStatusAPI(&testStatusProvider{}, socketPath)
	assert.ErrorContains(t, err, "not a unix socket")
}

// A stale socket from a previous daemon run must be replaced, not rejected.
func TestStatusAPIReplacesStaleSocket(t *testing.T) {
	socketPath := tempSocketPath(t)
	stale, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	// Keep the socket file behind so it looks like a crashed daemon's leftover.
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())

	api, err := newStatusAPI(&testStatusProvider{}, socketPath)
	require.NoError(t, err)
	_ = api.Stop(context.Background())
}
