// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

// === Mocks

// MockManager is a mock implementation of the manager.Manager interface.
type MockManager struct {
	mock.Mock
}

// AddHook is a mock implementation of the manager.Manager.AddHook method.
func (m *MockManager) AddHook(name string, probe *manager.Probe) error {
	args := m.Called(name, probe)
	return args.Error(0)
}

// DetachHook is a mock implementation of the manager.Manager.DetachHook method.
func (m *MockManager) DetachHook(probeID manager.ProbeIdentificationPair) error {
	args := m.Called(probeID)
	return args.Error(0)
}

// GetProbe is a mock implementation of the manager.Manager.GetProbe method.
func (m *MockManager) GetProbe(probeID manager.ProbeIdentificationPair) (*manager.Probe, bool) {
	args := m.Called(probeID)
	return args.Get(0).(*manager.Probe), args.Bool(1)
}

// MockFileRegistry is a mock implementation of the FileRegistry interface.
type MockFileRegistry struct {
	mock.Mock
}

// Register is a mock implementation of the FileRegistry.Register method.
func (m *MockFileRegistry) Register(namespacedPath string, pid uint32, activationCB, deactivationCB, alreadyRegistered utils.Callback) error {
	args := m.Called(namespacedPath, pid, activationCB, deactivationCB)
	return args.Error(0)
}

// Unregister is a mock implementation of the FileRegistry.Unregister method.
func (m *MockFileRegistry) Unregister(pid uint32) error {
	args := m.Called(pid)
	return args.Error(0)
}

// Clear is a mock implementation of the FileRegistry.Clear method.
func (m *MockFileRegistry) Clear() {
	m.Called()
}

// GetRegisteredProcesses is a mock implementation of the FileRegistry.GetRegisteredProcesses method.
func (m *MockFileRegistry) GetRegisteredProcesses() map[uint32]struct{} {
	args := m.Called()
	return args.Get(0).(map[uint32]struct{})
}

// MockBinaryInspector is a mock implementation of the BinaryInspector interface.
type MockBinaryInspector struct {
	mock.Mock
}

// Inspect is a mock implementation of the BinaryInspector.Inspect method.
func (m *MockBinaryInspector) Inspect(fpath utils.FilePath, requests []SymbolRequest) (map[string]bininspect.FunctionMetadata, error) {
	args := m.Called(fpath, requests)
	return args.Get(0).(map[string]bininspect.FunctionMetadata), args.Error(1)
}

// Cleanup is a mock implementation of the BinaryInspector.Cleanup method.
func (m *MockBinaryInspector) Cleanup(fpath utils.FilePath) {
	_ = m.Called(fpath)
}

type mockProcessMonitor struct {
	mock.Mock
}

func (m *mockProcessMonitor) SubscribeExec(cb func(uint32)) func() {
	args := m.Called(cb)
	return args.Get(0).(func())
}

func (m *mockProcessMonitor) SubscribeExit(cb func(uint32)) func() {
	args := m.Called(cb)
	return args.Get(0).(func())
}

// Create a new mockProcessMonitor that accepts any callback and returns a no-op function
func newMockProcessMonitor() *mockProcessMonitor {
	pm := &mockProcessMonitor{}
	pm.On("SubscribeExec", mock.Anything).Return(func() {})
	pm.On("SubscribeExit", mock.Anything).Return(func() {})

	return pm
}

// === Test utils

// FakeProcFSEntry represents a fake /proc filesystem entry for testing purposes.
type FakeProcFSEntry struct {
	Pid     uint32
	Cmdline string
	Command string
	Exe     string
	Maps    string
}

// CreateFakeProcFS creates a fake /proc filesystem with the given entries, useful for testing attachment to processes.
func CreateFakeProcFS(t *testing.T, entries []FakeProcFSEntry) string {
	procRoot := t.TempDir()

	for _, entry := range entries {
		baseDir := filepath.Join(procRoot, strconv.Itoa(int(entry.Pid)))

		createFile(t, filepath.Join(baseDir, "cmdline"), entry.Cmdline)
		createFile(t, filepath.Join(baseDir, "comm"), entry.Command)
		createFile(t, filepath.Join(baseDir, "maps"), entry.Maps)
		createSymlink(t, entry.Exe, filepath.Join(baseDir, "exe"))
	}

	return procRoot
}

func createFile(t *testing.T, path, data string) {
	if data == "" {
		return
	}

	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.WriteFile(path, []byte(data), 0775))
}

func createSymlink(t *testing.T, target, link string) {
	if target == "" {
		return
	}

	dir := filepath.Dir(link)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.Symlink(target, link))
}

func getLibSSLPath(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	libmmap := filepath.Join(curDir, "..", "..", "network", "usm", "testdata", "site-packages", "ddtrace")
	return filepath.Join(libmmap, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))
}

// SetRegistry allows changing the file registry used by the attacher. This is useful for testing purposes, to
// replace the registry with a mock object
func (ua *UprobeAttacher) SetRegistry(registry FileRegistry) {
	ua.fileRegistry = registry
}

// checkIfEventually checks a condition repeatedly until it returns true or a timeout is reached.
func checkIfEventually(condition func() bool, checkInterval time.Duration, checkTimeout time.Duration) bool {
	ch := make(chan bool, 1)

	timer := time.NewTimer(checkTimeout)
	defer timer.Stop()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for tick := ticker.C; ; {
		select {
		case <-timer.C:
			return false
		case <-tick:
			tick = nil
			go func() { ch <- condition() }()
		case v := <-ch:
			if v {
				return true
			}
			tick = ticker.C
		}
	}
}

// waitAndRetryIfFail is basically a way to do require.Eventually with multiple
// retries. In each retry, it will run the setupFunc, then wait until the
// condition defined by testFunc is met or the timeout is reached, and then run
// the retryCleanup function. The retryCleanup function is useful to clean up
// any state that was set up in the setupFunc. It will receive a boolean
// indicating if the test was successful or not, in case the cleanup needs to be
// different depending on the test result (e.g., if the test didn't fail we
// might want to keep some state). If the condition is met, it will return,
// otherwise it will retry the same thing again. If the condition is not met
// after maxRetries, it will fail the test.
func waitAndRetryIfFail(t *testing.T, setupFunc func(), testFunc func() bool, retryCleanup func(testSuccess bool), maxRetries int, checkInterval time.Duration, maxSingleCheckTime time.Duration, msgAndArgs ...interface{}) {
	for i := 0; i < maxRetries; i++ {
		if setupFunc != nil {
			setupFunc()
		}
		result := checkIfEventually(testFunc, checkInterval, maxSingleCheckTime)
		if retryCleanup != nil {
			retryCleanup(result)
		}

		if result {
			return
		}
	}

	require.Fail(t, "condition not met after %d retries", maxRetries, msgAndArgs)
}
