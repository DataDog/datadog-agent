// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package privilegedhelper

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func stubStart(t *testing.T, landlock int) *[]string {
	var execArgs []string
	originalLandlock, originalExec, originalLookup := landlockABIVersion, execve, lookupUser
	landlockABIVersion = func() (int, error) { return landlock, nil }
	execve = func(_ string, argv []string, _ []string) error {
		execArgs = argv
		return errors.New("exec stubbed")
	}
	// The test user stands in for dd-agent, which CI hosts do not have.
	lookupUser = func(string) (*user.User, error) { return user.Current() }
	t.Cleanup(func() { landlockABIVersion, execve, lookupUser = originalLandlock, originalExec, originalLookup })
	return &execArgs
}

func TestStartHelperSkipsUnsupportedKernels(t *testing.T) {
	execArgs := stubStart(t, minLandlockABI-1)
	socket := filepath.Join(t.TempDir(), "h.sock")

	require.NoError(t, startHelper(logmock.New(t), &helperParams{helperPath: "/bin/true", socketPath: socket}))

	assert.Nil(t, *execArgs, "the helper must not start")
	assert.NoFileExists(t, socket)
}

func TestStartHelperHandsOverTheSocket(t *testing.T) {
	execArgs := stubStart(t, minLandlockABI)
	// Keep the path short: Unix socket paths are limited to about 100 bytes.
	dir, err := os.MkdirTemp("", "rsh")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "h.sock")

	err = startHelper(logmock.New(t), &helperParams{helperPath: "/opt/rshell", socketPath: socket, policyPath: "/etc/policy.json"})
	require.ErrorContains(t, err, "exec stubbed")

	require.Len(t, *execArgs, 8)
	assert.Equal(t, []string{"/opt/rshell", "privileged-helper", "--policy=/etc/policy.json", "--idle-timeout=0"}, (*execArgs)[4:])
	var stat unix.Stat_t
	require.NoError(t, unix.Stat(socket, &stat))
	assert.Equal(t, uint32(unix.S_IFSOCK), stat.Mode&unix.S_IFMT)
	assert.Equal(t, uint32(0o600), stat.Mode&0o777, "only the owner may connect")
	assert.Equal(t, strconv.Itoa(os.Getuid()), strconv.Itoa(int(stat.Uid)))
}

func TestListenKeepsTheSocketOpenAcrossExec(t *testing.T) {
	dir, err := os.MkdirTemp("", "rsh")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "h.sock")
	// A stale socket from a previous run is replaced.
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	socket, err := listen(path, os.Getuid(), os.Getgid())
	require.NoError(t, err)
	t.Cleanup(func() { socket.Close() })

	flags, err := unix.FcntlInt(socket.Fd(), unix.F_GETFD, 0)
	require.NoError(t, err)
	assert.Zero(t, flags&unix.FD_CLOEXEC)
}
