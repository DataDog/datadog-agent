// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
)

func TestRegister(t *testing.T) {
	registerRecorder := new(CallbackRecorder)

	path, pathID := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := uint32(cmd.Process.Pid)

	r := newFileRegistry()
	require.NoError(t, r.Register(path, pid, registerRecorder.Callback(), IgnoreCB))

	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
	assert.Contains(t, r.GetRegisteredProcesses(), pid)
	assert.Equal(t, int64(1), r.telemetry.fileRegistered.Get())
}

func TestMultiplePIDsSharingSameFile(t *testing.T) {
	registerRecorder := new(CallbackRecorder)
	registerCallback := registerRecorder.Callback()

	unregisterRecorder := new(CallbackRecorder)
	unregisterCallback := unregisterRecorder.Callback()

	r := newFileRegistry()
	path, pathID := createTempTestFile(t, "foobar")

	cmd1, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	cmd2, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)

	pid1 := uint32(cmd1.Process.Pid)
	pid2 := uint32(cmd2.Process.Pid)

	// Trying to register the same file twice from different PIDs
	require.NoError(t, r.Register(path, pid1, registerCallback, unregisterCallback))
	require.Equal(t, ErrPathIsAlreadyRegistered, r.Register(path, pid2, registerCallback, unregisterCallback))

	// Assert that the callback should execute only *once*
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))

	// Assert that the two PIDs are being tracked
	assert.Contains(t, r.GetRegisteredProcesses(), pid1)
	assert.Contains(t, r.GetRegisteredProcesses(), pid2)

	// Assert that the first call to `Unregister` (from pid1) doesn't trigger
	// the callback but removes pid1 from the list
	require.NoError(t, r.Unregister(pid1))
	assert.Equal(t, 0, unregisterRecorder.CallsForPathID(pathID))
	assert.NotContains(t, r.GetRegisteredProcesses(), pid1)
	assert.Contains(t, r.GetRegisteredProcesses(), pid2)

	// After the second call to Unregister` we should trigger the callback
	// because there are no longer PIDs pointing to this file
	require.NoError(t, r.Unregister(pid2))
	assert.Equal(t, 1, unregisterRecorder.CallsForPathID(pathID))
	assert.NotContains(t, r.GetRegisteredProcesses(), pid1)
	assert.NotContains(t, r.GetRegisteredProcesses(), pid2)

	// Telemetry assertions
	assert.Equal(t, int64(1), r.telemetry.fileRegistered.Get())
	assert.Equal(t, int64(1), r.telemetry.fileAlreadyRegistered.Get())
	assert.Equal(t, int64(1), r.telemetry.fileUnregistered.Get())
}

func TestRepeatedRegistrationsFromSamePID(t *testing.T) {
	registerRecorder := new(CallbackRecorder)
	registerCallback := registerRecorder.Callback()

	unregisterRecorder := new(CallbackRecorder)
	unregisterCallback := unregisterRecorder.Callback()

	r := newFileRegistry()
	path, pathID := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := uint32(cmd.Process.Pid)

	require.NoError(t, r.Register(path, pid, registerCallback, unregisterCallback))
	require.Equal(t, ErrPathIsAlreadyRegistered, r.Register(path, pid, registerCallback, unregisterCallback))
	require.NoError(t, r.Unregister(pid))

	// Assert that despite multiple calls to `Register` from the same PID we
	// only need a single call to `Unregister` to trigger the Unregister callback
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
	assert.Equal(t, 1, unregisterRecorder.CallsForPathID(pathID))
	assert.NotContains(t, r.GetRegisteredProcesses(), pid)
}

func TestFailedRegistration(t *testing.T) {
	// Create a callback recorder that returns an error on purpose
	registerRecorder := new(CallbackRecorder)
	registerRecorder.ReturnError = fmt.Errorf("failed registration")
	registerCallback := registerRecorder.Callback()

	r := newFileRegistry()
	path, pathID := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := uint32(cmd.Process.Pid)

	require.NoError(t, r.Register(path, pid, registerCallback, IgnoreCB))

	// First let's assert that the callback was executed once, but there are no
	// registered processes because the registration should have failed
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
	assert.Empty(t, r.GetRegisteredProcesses())

	// Now let's try to register the same process again
	require.Equal(t, errPathIsBlocked, r.Register(path, pid, registerCallback, IgnoreCB))

	// Assert that the number of callback executions hasn't changed for this pathID
	// This is because we have block-listed this file
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
}

func TestFilePathInCallbackArgument(t *testing.T) {
	var capturedPath string
	callback := func(f FilePath) error {
		capturedPath = f.HostPath
		return nil
	}

	path, _ := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := cmd.Process.Pid

	r := newFileRegistry()
	require.NoError(t, r.Register(path, uint32(pid), callback, callback))

	// Assert that the callback paths match the pattern <proc_root>/<pid>/root/<path>
	expectedPath := filepath.Join(r.procRoot, strconv.Itoa(pid), "root", path)
	assert.Equal(t, expectedPath, capturedPath)
}

func TestRelativeFilePathInCallbackArgument(t *testing.T) {
	var capturedPath string
	callback := func(f FilePath) error {
		capturedPath = f.HostPath
		return nil
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)

	// If cwd has symlinks, then the result of `filepath.Rel` below is not
	// necessarily a valid path.
	cwd, err = filepath.EvalSymlinks(cwd)
	require.NoError(t, err)

	path, _ := createTempTestFile(t, "foobar")

	relpath, err := filepath.Rel(cwd, path)
	require.NoError(t, err)

	cmd, err := testutil.OpenFromAnotherProcess(t, relpath)
	require.NoError(t, err)
	pid := cmd.Process.Pid

	r := newFileRegistry()
	require.NoError(t, r.Register(relpath, uint32(pid), callback, callback))

	// Assert that the callback paths match the pattern <proc_root>/<pid>/cwd/<path>.
	// We need to avoid `filepath.Join` for the last component since using
	// that would `Clean` the path, removing the relative components.
	expectedPath := filepath.Join(r.procRoot, strconv.Itoa(pid), "cwd") + string(filepath.Separator) + relpath
	assert.Equal(t, expectedPath, capturedPath)
}

func createTempTestFile(t *testing.T, name string) (string, PathIdentifier) {
	path := filepath.Join(t.TempDir(), name)

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	pathID, err := NewPathIdentifier(path)
	require.NoError(t, err)

	return path, pathID
}

func newFileRegistry() *FileRegistry {
	// Ensure that tests relying on telemetry data will always have a clean slate
	telemetry.Clear()
	return NewFileRegistry("")
}
