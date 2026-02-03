// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package monitor

import (
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	netnsutil "github.com/DataDog/datadog-agent/pkg/util/kernel/netns"
)

func getProcessMonitor(t *testing.T) *ProcessMonitor {
	pm := GetProcessMonitor()
	t.Cleanup(func() {
		pm.Stop()
		telemetry.Clear()
	})
	return pm
}

// pidRecorder is a helper to record pids and check if they were recorded.
type pidRecorder struct {
	mu   sync.RWMutex
	pids map[uint32]struct{}
}

// newPidRecorder creates a new pidRecorder.
func newPidRecorder() *pidRecorder {
	return &pidRecorder{pids: make(map[uint32]struct{})}
}

// record records a pid.
func (pr *pidRecorder) record(pid uint32) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.pids[pid] = struct{}{}
}

// has checks if a pid was recorded.
func (pr *pidRecorder) has(pid uint32) bool {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	_, ok := pr.pids[pid]
	return ok
}

// getProcessCallback returns a ProcessCallback wrapper of the pidRecorder.record method.
func getProcessCallback(r *pidRecorder) *ProcessCallback {
	f := func(pid uint32) {
		r.record(pid)
	}
	return &f
}

func waitForProcessMonitor(t *testing.T, pm *ProcessMonitor) {
	execRecorder := newPidRecorder()
	registerCallback(t, pm, true, getProcessCallback(execRecorder))

	exitRecorder := newPidRecorder()
	registerCallback(t, pm, false, getProcessCallback(exitRecorder))

	const (
		iterationInterval = 100 * time.Millisecond
		iterations        = 10
	)

	// Trying for 10 seconds (100 iterations * 100ms) to capture exec and exit events.
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		cmd := exec.Command("/bin/echo")
		require.NoError(ct, cmd.Run())
		require.NotZero(ct, cmd.Process.Pid)
		t.Logf("running %d", cmd.Process.Pid)
		// Trying for a second (10 iterations * 100ms) to capture exec and exit events.
		// If we failed, try to run the command again.
		require.EventuallyWithT(ct, func(innerCt *assert.CollectT) {
			require.Truef(innerCt, execRecorder.has(uint32(cmd.Process.Pid)), "didn't capture exec event %d", cmd.Process.Pid)
			require.True(innerCt, exitRecorder.has(uint32(cmd.Process.Pid)), "didn't capture exit event %d", cmd.Process.Pid)
		}, iterations*iterationInterval, iterationInterval)
	}, iterations*iterations*iterationInterval, iterationInterval)
}

func initializePM(t *testing.T, pm *ProcessMonitor, useEventStream bool) {
	require.NoError(t, pm.Initialize(useEventStream))
	if useEventStream {
		InitializeEventConsumer(testutil.NewTestProcessConsumer(t))
	}
	waitForProcessMonitor(t, pm)
}

func registerCallback(t *testing.T, pm *ProcessMonitor, isExec bool, callback *ProcessCallback) func() {
	registrationFunc := pm.SubscribeExit
	if isExec {
		registrationFunc = pm.SubscribeExec
	}
	unsubscribe := registrationFunc(*callback)
	t.Cleanup(unsubscribe)
	return unsubscribe
}

func TestProcessMonitorSingleton(t *testing.T) {
	// Making sure we get the same process monitor if we call it twice.
	pm := getProcessMonitor(t)
	pm2 := getProcessMonitor(t)

	require.Equal(t, pm, pm2)
}

type processMonitorSuite struct {
	suite.Suite
	useEventStream bool
}

func (s *processMonitorSuite) TestProcessMonitorSanity() {
	t := s.T()
	pm := getProcessMonitor(t)

	execRecorder := newPidRecorder()
	registerCallback(t, pm, true, getProcessCallback(execRecorder))

	exitRecorder := newPidRecorder()
	registerCallback(t, pm, false, getProcessCallback(exitRecorder))

	initializePM(t, pm, s.useEventStream)
	cmd := exec.Command("/bin/echo", "test")
	require.NoError(t, cmd.Run())
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.Truef(ct, execRecorder.has(uint32(cmd.Process.Pid)), "didn't capture exec event %d", cmd.Process.Pid)
		assert.Truef(ct, exitRecorder.has(uint32(cmd.Process.Pid)), "didn't capture exit event %d", cmd.Process.Pid)
	}, 5*time.Second, time.Millisecond*100)

	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exec.Get(), "events is not >= than exec")
	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exit.Get(), "events is not >= than exit")
	require.NotEqual(t, int64(0), pm.tel.exec.Get())
	require.NotEqual(t, int64(0), pm.tel.exit.Get())
	require.Equal(t, int64(0), pm.tel.restart.Get())
	require.Equal(t, int64(0), pm.tel.reinitFailed.Get())
	require.Equal(t, int64(0), pm.tel.processScanFailed.Get())
	require.GreaterOrEqual(t, pm.tel.callbackExecuted.Get(), int64(1), "callback_executed")
}

func TestProcessMonitor(t *testing.T) {
	t.Run("netlink", func(t *testing.T) {
		suite.Run(t, &processMonitorSuite{useEventStream: false})
	})
	t.Run("event stream", func(t *testing.T) {
		suite.Run(t, &processMonitorSuite{useEventStream: true})
	})
}

func (s *processMonitorSuite) TestProcessRegisterMultipleCallbacks() {
	t := s.T()
	pm := getProcessMonitor(t)

	const iterations = 10
	execs := make([]*pidRecorder, iterations)
	exits := make([]*pidRecorder, iterations)
	for i := 0; i < iterations; i++ {
		newExecRecorder := newPidRecorder()
		registerCallback(t, pm, true, getProcessCallback(newExecRecorder))
		execs[i] = newExecRecorder

		newExitRecorder := newPidRecorder()
		registerCallback(t, pm, false, getProcessCallback(newExitRecorder))
		exits[i] = newExitRecorder
	}

	initializePM(t, pm, s.useEventStream)
	cmd := exec.Command("/bin/sleep", "1")
	require.NoError(t, cmd.Run())
	require.EventuallyWithTf(t, func(ct *assert.CollectT) {
		// Instead of breaking immediately when we don't find the event, we want logs to be printed for all iterations.
		for i := 0; i < iterations; i++ {
			assert.Truef(ct, execs[i].has(uint32(cmd.Process.Pid)), "iter %d didn't capture exec event %d", i, cmd.Process.Pid)
			assert.Truef(ct, exits[i].has(uint32(cmd.Process.Pid)), "iter %d didn't capture exit event %d", i, cmd.Process.Pid)
		}
	}, 5*time.Second, 100*time.Millisecond, "at least of the callbacks didn't capture events")

	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exec.Get(), "events is not >= than exec")
	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exit.Get(), "events is not >= than exit")
	require.NotEqual(t, int64(0), pm.tel.exec.Get())
	require.NotEqual(t, int64(0), pm.tel.exit.Get())
	require.Equal(t, int64(0), pm.tel.restart.Get())
	require.Equal(t, int64(0), pm.tel.reinitFailed.Get())
	require.Equal(t, int64(0), pm.tel.processScanFailed.Get())
	require.GreaterOrEqual(t, pm.tel.callbackExecuted.Get(), int64(1), "callback_executed")
}

func TestProcessMonitorRefcount(t *testing.T) {
	var pm *ProcessMonitor

	for i := 1; i <= 10; i++ {
		pm = GetProcessMonitor()
		require.Equal(t, pm.refcount.Load(), int32(i))
	}

	for i := 1; i <= 10; i++ {
		pm.Stop()
		require.Equal(t, pm.refcount.Load(), int32(10-i))
	}
}

func (s *processMonitorSuite) TestProcessMonitorInNamespace() {
	t := s.T()

	pm := getProcessMonitor(t)

	execRecorder := newPidRecorder()
	registerCallback(t, pm, true, getProcessCallback(execRecorder))

	exitRecorder := newPidRecorder()
	registerCallback(t, pm, false, getProcessCallback(exitRecorder))

	monNs, err := netns.New()
	require.NoError(t, err, "could not create network namespace for process monitor")
	t.Cleanup(func() { monNs.Close() })

	require.NoError(t, netnsutil.WithNS(monNs, func() error {
		initializePM(t, pm, s.useEventStream)
		return nil
	}), "could not start process monitor in netNS")
	t.Cleanup(pm.Stop)

	time.Sleep(500 * time.Millisecond)
	// Process in root NS
	cmd := exec.Command("/bin/sleep", "1")
	require.NoError(t, cmd.Run(), "could not run process in root namespace")
	pid := uint32(cmd.ProcessState.Pid())

	require.EventuallyWithTf(t, func(ct *assert.CollectT) {
		assert.Truef(ct, execRecorder.has(pid), "didn't capture exec event %d", pid)
		assert.Truef(ct, exitRecorder.has(pid), "didn't capture exit event %d", pid)
	}, 5*time.Second, 100*time.Millisecond, "did not capture process EXEC/EXIT from root namespace")

	// Process in another NS
	cmdNs, err := netns.New()
	require.NoError(t, err, "could not create network namespace for process")
	defer cmdNs.Close()

	cmd = exec.Command("/bin/sleep", "1")
	require.NoError(t, netnsutil.WithNS(cmdNs, cmd.Run), "could not run process in other network namespace")
	pid = uint32(cmd.ProcessState.Pid())

	require.EventuallyWithTf(t, func(ct *assert.CollectT) {
		assert.Truef(ct, execRecorder.has(pid), "didn't capture exec event %d", pid)
		assert.Truef(ct, exitRecorder.has(pid), "didn't capture exit event %d", pid)
	}, 5*time.Second, 100*time.Millisecond, "did not capture process EXEC/EXIT from root namespace")

	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exec.Get(), "events is not >= than exec")
	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exit.Get(), "events is not >= than exit")
	require.NotEqual(t, int64(0), pm.tel.exec.Get())
	require.NotEqual(t, int64(0), pm.tel.exit.Get())
	require.Equal(t, int64(0), pm.tel.restart.Get())
	require.Equal(t, int64(0), pm.tel.reinitFailed.Get())
	require.Equal(t, int64(0), pm.tel.processScanFailed.Get())
	require.GreaterOrEqual(t, pm.tel.callbackExecuted.Get(), int64(1), "callback_executed")
}
