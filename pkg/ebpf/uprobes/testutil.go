// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package uprobes

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	consumertestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
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
func (m *MockFileRegistry) Register(namespacedPath string, pid uint32, activationCB, deactivationCB, _ utils.Callback) error {
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

// Log is a mock implementation of the FileRegistry.Log method.
func (m *MockFileRegistry) Log() {
	m.Called()
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
	Env     map[string]string
}

// getEnvironContents returns the formatted contents of the /proc/<pid>/environ file for the entry.
func (f *FakeProcFSEntry) getEnvironContents() string {
	if len(f.Env) == 0 {
		return ""
	}

	formattedEnvVars := make([]string, 0, len(f.Env))
	for k, v := range f.Env {
		formattedEnvVars = append(formattedEnvVars, fmt.Sprintf("%s=%s", k, v))
	}

	return strings.Join(formattedEnvVars, "\x00") + "\x00"
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
		createFile(t, filepath.Join(baseDir, "environ"), entry.getEnvironContents())
	}

	return procRoot
}

func createFile(t *testing.T, path, data string) {
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

	extraFmt := ""
	if len(msgAndArgs) > 0 {
		extraFmt = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...) + ": "
	}

	require.Fail(t, "condition not met", "%scondition not met after %d retries", extraFmt, maxRetries)
}

// processMonitorProxy is a wrapper around a ProcessMonitor that stores the
// callbacks subscribed to it, and triggers them which allows manually
// triggering the callbacks for testing purposes.
type processMonitorProxy struct {
	target        ProcessMonitor
	mutex         sync.Mutex // performance is not a worry for this, so use a single mutex for simplicity
	execCallbacks map[*func(uint32)]struct{}
	exitCallbacks map[*func(uint32)]struct{}
}

// ensure it implements the ProcessMonitor interface
var _ ProcessMonitor = &processMonitorProxy{}

func newProcessMonitorProxy(target ProcessMonitor) *processMonitorProxy {
	return &processMonitorProxy{
		target:        target,
		execCallbacks: make(map[*func(uint32)]struct{}),
		exitCallbacks: make(map[*func(uint32)]struct{}),
	}
}

func (o *processMonitorProxy) SubscribeExec(cb func(uint32)) func() {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.execCallbacks[&cb] = struct{}{}

	return o.target.SubscribeExec(cb)
}

func (o *processMonitorProxy) SubscribeExit(cb func(uint32)) func() {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.exitCallbacks[&cb] = struct{}{}

	return o.target.SubscribeExit(cb)
}

func (o *processMonitorProxy) triggerExit(pid uint32) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	for cb := range o.exitCallbacks {
		(*cb)(pid)
	}
}

// Reset resets the state of the processMonitorProxy, removing all callbacks.
func (o *processMonitorProxy) Reset() {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	o.execCallbacks = make(map[*func(uint32)]struct{})
	o.exitCallbacks = make(map[*func(uint32)]struct{})
}

func launchProcessMonitor(t *testing.T, useEventStream bool) *monitor.ProcessMonitor {
	pm := monitor.GetProcessMonitor()
	t.Cleanup(pm.Stop)
	require.NoError(t, pm.Initialize(useEventStream))
	if useEventStream {
		monitor.InitializeEventConsumer(consumertestutil.NewTestProcessConsumer(t))
	}

	return pm
}
