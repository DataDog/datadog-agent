// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMountInfoPidPath(t *testing.T) {
	// Reset to ensure we're using /proc
	resetProcFSRoot()
	defer resetProcFSRoot()

	path := MountInfoPidPath(1234)
	assert.Contains(t, path, "1234")
	assert.Contains(t, path, "mountinfo")
}

func TestProcFSRootDefault(t *testing.T) {
	// Save and restore env vars
	origHostProc := os.Getenv("HOST_PROC")
	origDockerDDAgent := os.Getenv("DOCKER_DD_AGENT")
	defer func() {
		os.Setenv("HOST_PROC", origHostProc)
		os.Setenv("DOCKER_DD_AGENT", origDockerDDAgent)
	}()

	// Clear env vars and reset memoization
	os.Unsetenv("HOST_PROC")
	os.Unsetenv("DOCKER_DD_AGENT")
	resetProcFSRoot()

	root := ProcFSRoot()
	assert.Equal(t, "/proc", root)
}

func TestProcFSRootFromEnv(t *testing.T) {
	// Save and restore env vars
	origHostProc := os.Getenv("HOST_PROC")
	defer func() {
		os.Setenv("HOST_PROC", origHostProc)
		resetProcFSRoot()
	}()

	os.Setenv("HOST_PROC", "/custom/proc")
	resetProcFSRoot()

	root := ProcFSRoot()
	assert.Equal(t, "/custom/proc", root)
}

func TestHostProc(t *testing.T) {
	// Save and restore env vars
	origHostProc := os.Getenv("HOST_PROC")
	defer func() {
		os.Setenv("HOST_PROC", origHostProc)
		resetProcFSRoot()
	}()

	os.Unsetenv("HOST_PROC")
	resetProcFSRoot()

	// Test without additional path components
	path := HostProc()
	assert.Equal(t, "/proc", path)

	// Test with additional path components
	path = HostProc("self", "fd")
	assert.Equal(t, "/proc/self/fd", path)
}

func TestHostSys(t *testing.T) {
	// Test without additional path components
	path := HostSys()
	assert.NotEmpty(t, path)

	// Test with additional path components
	path = HostSys("class", "net")
	assert.Contains(t, path, filepath.Join("class", "net"))
}

func TestHostBoot(t *testing.T) {
	// Test without additional path components
	path := HostBoot()
	assert.NotEmpty(t, path)

	// Test with additional path components
	path = HostBoot("config-5.4.0")
	assert.Contains(t, path, "config-5.4.0")
}

func TestParseMountInfoFile(t *testing.T) {
	// Test with current process - should work on Linux
	mounts, err := ParseMountInfoFile(int32(os.Getpid()))
	require.NoError(t, err)
	assert.NotEmpty(t, mounts, "should have at least some mounts")
}

func TestParseMountInfoFileInvalidPid(t *testing.T) {
	// Test with invalid PID
	_, err := ParseMountInfoFile(-999999)
	assert.Error(t, err)
}
