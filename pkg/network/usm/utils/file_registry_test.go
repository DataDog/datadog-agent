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

func verifyPIDInFilePath(t *testing.T) func(FilePath) error {
	return func(f FilePath) error {
		require.NotZerof(t, f.PID, "PID should not be zero")
		return nil
	}
}

// TestClearSanity tests the behavior and cleanup of Clear method.
func TestClearSanity(t *testing.T) {
	r := newFileRegistry()

	path1, pathID1 := createTempTestFile(t, "foobar")
	path2, pathID2 := createTempTestFile(t, "foobar2")

	path1Pids := make([]uint32, 0)
	for i := 0; i < 3; i++ {
		target := fmt.Sprintf("%s-%d", path1, i)
		createSymlink(t, path1, target)
		cmd, err := testutil.OpenFromAnotherProcess(t, target)
		require.NoError(t, err)
		path1Pids = append(path1Pids, uint32(cmd.Process.Pid))
		_ = r.Register(target, uint32(cmd.Process.Pid), verifyPIDInFilePath(t), verifyPIDInFilePath(t), IgnoreCB)
	}
	path2Pids := make([]uint32, 0)
	for i := 0; i < 2; i++ {
		target := fmt.Sprintf("%s-%d", path2, i)
		createSymlink(t, path2, target)
		cmd, err := testutil.OpenFromAnotherProcess(t, target)
		require.NoError(t, err)
		path2Pids = append(path2Pids, uint32(cmd.Process.Pid))
		_ = r.Register(target, uint32(cmd.Process.Pid), verifyPIDInFilePath(t), verifyPIDInFilePath(t), IgnoreCB)
	}

	assert.Len(t, r.byPID, len(path1Pids)+len(path2Pids))
	for _, pid := range path1Pids {
		assert.Contains(t, r.byPID, pid)
	}
	for _, pid := range path2Pids {
		assert.Contains(t, r.byPID, pid)
	}
	assert.Len(t, r.byID, 2)
	assert.Contains(t, r.byID, pathID1)
	assert.Contains(t, r.byID, pathID2)

	r.Clear()

	// Verify empty registry
	assert.Empty(t, r.byPID)
	assert.Empty(t, r.byID)
	assert.True(t, r.stopped)
}

// TestClearLeakedPathID tests that the registry can handle a leaked pathID, that does not have a corresponding PID.
func TestClearLeakedPathID(t *testing.T) {
	r := newFileRegistry()

	path1, pathID1 := createTempTestFile(t, "foobar")
	path2, pathID2 := createTempTestFile(t, "foobar2")

	path1Pids := make([]uint32, 0)
	for i := 0; i < 3; i++ {
		target := fmt.Sprintf("%s-%d", path1, i)
		createSymlink(t, path1, target)
		cmd, err := testutil.OpenFromAnotherProcess(t, target)
		require.NoError(t, err)
		path1Pids = append(path1Pids, uint32(cmd.Process.Pid))
		_ = r.Register(target, uint32(cmd.Process.Pid), verifyPIDInFilePath(t), verifyPIDInFilePath(t), IgnoreCB)
	}

	assert.Len(t, r.byPID, len(path1Pids))
	for _, pid := range path1Pids {
		assert.Contains(t, r.byPID, pid)
	}
	assert.Len(t, r.byID, 1)
	assert.Contains(t, r.byID, pathID1)

	r.byID[pathID2] = r.newRegistration(path2, func(f FilePath) error {
		require.Zerof(t, f.PID, "PID should be zero, as there was no PID associated with this pathID")
		return nil
	})
	assert.Contains(t, r.byID, pathID2)

	r.Clear()

	// Verify empty registry
	assert.Empty(t, r.byPID)
	assert.Empty(t, r.byID)
	assert.True(t, r.stopped)
}

func TestRegister(t *testing.T) {
	registerRecorder := new(CallbackRecorder)

	path, pathID := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := uint32(cmd.Process.Pid)

	r := newFileRegistry()
	require.NoError(t, r.Register(path, pid, registerRecorder.Callback(), IgnoreCB, IgnoreCB))

	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
	assert.Contains(t, r.GetRegisteredProcesses(), pid)
	assert.Equal(t, int64(1), r.telemetry.fileRegistered.Get())
}

func TestMultiplePIDsSharingSameFile(t *testing.T) {
	registerRecorder := new(CallbackRecorder)
	registerCallback := registerRecorder.Callback()

	unregisterRecorder := new(CallbackRecorder)
	unregisterCallback := unregisterRecorder.Callback()

	alreadyRegisteredRecorder := new(CallbackRecorder)
	alreadyRegisteredCallback := alreadyRegisteredRecorder.Callback()

	r := newFileRegistry()
	path, pathID := createTempTestFile(t, "foobar")

	cmd1, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	cmd2, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)

	pid1 := uint32(cmd1.Process.Pid)
	pid2 := uint32(cmd2.Process.Pid)

	// Trying to register the same file twice from different PIDs
	require.NoError(t, r.Register(path, pid1, registerCallback, unregisterCallback, alreadyRegisteredCallback))
	require.Equal(t, ErrPathIsAlreadyRegistered, r.Register(path, pid2, registerCallback, unregisterCallback, alreadyRegisteredCallback))

	// Assert that the callback should execute only *once*
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))

	// Assert that the two PIDs are being tracked
	assert.Contains(t, r.GetRegisteredProcesses(), pid1)
	assert.Contains(t, r.GetRegisteredProcesses(), pid2)

	// Assert that the callback should execute only *once*
	assert.Equal(t, 1, alreadyRegisteredRecorder.CallsForPathID(pathID))

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

	require.NoError(t, r.Register(path, pid, registerCallback, unregisterCallback, IgnoreCB))
	require.Equal(t, ErrPathIsAlreadyRegistered, r.Register(path, pid, registerCallback, unregisterCallback, IgnoreCB))
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

	unregisterRecorder := new(CallbackRecorder)
	unregisterCallback := unregisterRecorder.Callback()

	r := newFileRegistry()
	path, pathID := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := uint32(cmd.Process.Pid)

	err = r.Register(path, pid, registerCallback, unregisterCallback, IgnoreCB)
	require.ErrorIs(t, err, registerRecorder.ReturnError)

	// First let's assert that the callback was executed once, but there are no
	// registered processes because the registration should have failed
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
	assert.Empty(t, r.GetRegisteredProcesses())

	// The unregister callback should have been called to clean up the failed registration.
	assert.Equal(t, 1, unregisterRecorder.CallsForPathID(pathID))

	// Now let's try to register the same process again
	require.Equal(t, errPathIsBlocked, r.Register(path, pid, registerCallback, IgnoreCB, IgnoreCB))

	// Assert that the number of callback executions hasn't changed for this pathID
	// This is because we have block-listed this file
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))

	assert.Contains(t, debugger.GetBlockedPathIDs(""), pathID)
	debugger.ClearBlocked()
	assert.Empty(t, debugger.GetBlockedPathIDs(""))
}

func TestShortLivedProcess(t *testing.T) {
	// Create a callback recorder that returns an error on purpose
	registerRecorder := new(CallbackRecorder)
	registerRecorder.ReturnError = fmt.Errorf("failed registration")
	recorderCallback := registerRecorder.Callback()

	unregisterRecorder := new(CallbackRecorder)
	unregisterCallback := unregisterRecorder.Callback()

	r := newFileRegistry()
	path, pathID := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := uint32(cmd.Process.Pid)

	registerCallback := func(fp FilePath) error {
		// Simulate a short-lived process by killing it during the registration.
		cmd.Process.Kill()
		cmd.Process.Wait()
		return recorderCallback(fp)
	}

	err = r.Register(path, pid, registerCallback, unregisterCallback, IgnoreCB)
	require.ErrorIs(t, err, registerRecorder.ReturnError)

	// First let's assert that the callback was executed once, but there are no
	// registered processes because the registration should have failed
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
	assert.Empty(t, r.GetRegisteredProcesses())

	// The unregister callback should have been called to clean up the failed registration.
	assert.Equal(t, 1, unregisterRecorder.CallsForPathID(pathID))

	cmd, err = testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid = uint32(cmd.Process.Pid)

	registerRecorder.ReturnError = nil

	// Now let's try to register the same path again
	require.Nil(t, r.Register(path, pid, recorderCallback, IgnoreCB, IgnoreCB))

	// Assert that the path is successfully registered since it shouldn't have been blocked.
	assert.Equal(t, 2, registerRecorder.CallsForPathID(pathID))
	assert.Contains(t, r.GetRegisteredProcesses(), pid)
}

func TestNoBlockErrEnvironment(t *testing.T) {
	registerRecorder := new(CallbackRecorder)
	registerRecorder.ReturnError = fmt.Errorf("%w: failed registration", ErrEnvironment)
	registerCallback := registerRecorder.Callback()

	unregisterRecorder := new(CallbackRecorder)
	unregisterCallback := unregisterRecorder.Callback()

	r := newFileRegistry()
	path, pathID := createTempTestFile(t, "foobar")
	cmd, err := testutil.OpenFromAnotherProcess(t, path)
	require.NoError(t, err)
	pid := uint32(cmd.Process.Pid)

	err = r.Register(path, pid, registerCallback, unregisterCallback, IgnoreCB)
	require.ErrorIs(t, err, registerRecorder.ReturnError)

	// First let's assert that the callback was executed once, but there are no
	// registered processes because the registration should have failed
	assert.Equal(t, 1, registerRecorder.CallsForPathID(pathID))
	assert.Empty(t, r.GetRegisteredProcesses())

	// The unregister callback should have been called to clean up the failed registration.
	assert.Equal(t, 1, unregisterRecorder.CallsForPathID(pathID))

	registerRecorder.ReturnError = nil

	// Now let's try to register the same path again
	require.Nil(t, r.Register(path, pid, registerCallback, IgnoreCB, IgnoreCB))

	// Assert that the path is successfully registered since it shouldn't have
	// been blocked since we retruned an error wrapping ErrEnvironment.
	assert.Equal(t, 2, registerRecorder.CallsForPathID(pathID))
	assert.Contains(t, r.GetRegisteredProcesses(), pid)
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
	require.NoError(t, r.Register(path, uint32(pid), callback, callback, IgnoreCB))

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
	require.NoError(t, r.Register(relpath, uint32(pid), callback, callback, IgnoreCB))

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

func createSymlink(t *testing.T, old, new string) {
	require.NoError(t, os.Symlink(old, new))
	t.Cleanup(func() { require.NoError(t, os.Remove(new)) })
}

func newFileRegistry() *FileRegistry {
	// Ensure that tests relying on telemetry data will always have a clean slate
	telemetry.Clear()
	ResetDebugger()
	return NewFileRegistry("")
}
