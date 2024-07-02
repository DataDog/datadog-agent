// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package monitor

import (
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/vishvananda/netns"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	eventmonitortestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getProcessMonitor(t *testing.T) *ProcessMonitor {
	pm := GetProcessMonitor()
	t.Cleanup(func() {
		pm.Stop()
		telemetry.Clear()
	})
	return pm
}

func initializePM(t *testing.T, pm *ProcessMonitor, useEventStream bool) {
	require.NoError(t, pm.Initialize(useEventStream))
	if useEventStream {
		eventmonitortestutil.StartEventMonitor(t, func(t *testing.T, evm *eventmonitor.EventMonitor) {
			// Can't use the implementation in procmontestutil due to import cycles
			procmonconsumer, err := NewProcessMonitorEventConsumer(evm)
			require.NoError(t, err)
			evm.RegisterEventConsumer(procmonconsumer)
			log.Info("process monitoring test consumer initialized")
		})
	}
	time.Sleep(time.Millisecond * 500)
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

func getTestBinaryPath(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "echo")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})
	require.NoError(t, util.CopyFile("/bin/echo", tmpFile.Name()))

	return tmpFile.Name()
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
	numberOfExecs := atomic.Int32{}
	testBinaryPath := getTestBinaryPath(t)
	callback := func(pid uint32) { numberOfExecs.Inc() }
	registerCallback(t, pm, true, (*ProcessCallback)(&callback))

	numberOfExits := atomic.Int32{}
	exitCallback := func(pid uint32) { numberOfExits.Inc() }
	registerCallback(t, pm, false, (*ProcessCallback)(&exitCallback))

	initializePM(t, pm, s.useEventStream)
	require.NoError(t, exec.Command(testBinaryPath, "test").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs.Load() > 1 && numberOfExits.Load() > 0
	}, time.Second, time.Millisecond*200, "didn't capture exec/exit events %d", numberOfExecs.Load())

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

func (s *processMonitorSuite) TestProcessRegisterMultipleExecCallbacks() {
	t := s.T()
	pm := getProcessMonitor(t)

	const iterations = 10
	counters := make([]*atomic.Int32, iterations)
	for i := 0; i < iterations; i++ {
		counters[i] = &atomic.Int32{}
		c := counters[i]
		callback := func(pid uint32) { c.Inc() }
		registerCallback(t, pm, true, (*ProcessCallback)(&callback))
	}

	initializePM(t, pm, s.useEventStream)
	require.NoError(t, exec.Command("/bin/echo").Run())
	require.Eventuallyf(t, func() bool {
		for i := 0; i < iterations; i++ {
			if counters[i].Load() <= int32(0) {
				t.Logf("iter %d didn't capture event", i)
				return false
			}
		}
		return true
	}, time.Second, time.Millisecond*200, "at least of the callbacks didn't capture events")
}

func (s *processMonitorSuite) TestProcessRegisterMultipleExitCallbacks() {
	t := s.T()
	pm := getProcessMonitor(t)

	const iterations = 10
	counters := make([]*atomic.Int32, iterations)
	exitCounters := make([]*atomic.Int32, iterations)
	for i := 0; i < iterations; i++ {
		counters[i] = &atomic.Int32{}
		c := counters[i]
		// Sanity subscribing a callback.
		callback := func(pid uint32) { c.Inc() }
		registerCallback(t, pm, true, (*ProcessCallback)(&callback))

		exitCounters[i] = &atomic.Int32{}
		exitc := exitCounters[i]
		// Sanity subscribing a callback.
		exitCallback := func(pid uint32) { exitc.Inc() }
		registerCallback(t, pm, false, (*ProcessCallback)(&exitCallback))
	}

	initializePM(t, pm, s.useEventStream)
	require.NoError(t, exec.Command("/bin/echo").Run())
	require.Eventuallyf(t, func() bool {
		for i := 0; i < iterations; i++ {
			if counters[i].Load() <= int32(0) {
				t.Logf("iter %d didn't capture exec event", i)
				return false
			}
			if exitCounters[i].Load() <= int32(0) {
				t.Logf("iter %d didn't capture exit event", i)
				return false
			}
		}
		return true
	}, time.Second, time.Millisecond*200, "at least of the callbacks didn't capture events")

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
	execSet := sync.Map{}
	exitSet := sync.Map{}

	pm := getProcessMonitor(t)

	callback := func(pid uint32) { execSet.Store(pid, struct{}{}) }
	registerCallback(t, pm, true, (*ProcessCallback)(&callback))

	exitCallback := func(pid uint32) { exitSet.Store(pid, struct{}{}) }
	registerCallback(t, pm, false, (*ProcessCallback)(&exitCallback))

	monNs, err := netns.New()
	require.NoError(t, err, "could not create network namespace for process monitor")
	t.Cleanup(func() { monNs.Close() })

	require.NoError(t, kernel.WithNS(monNs, func() error {
		initializePM(t, pm, s.useEventStream)
		return nil
	}), "could not start process monitor in netNS")
	t.Cleanup(pm.Stop)

	time.Sleep(500 * time.Millisecond)
	// Process in root NS
	cmd := exec.Command("/bin/echo")
	require.NoError(t, cmd.Run(), "could not run process in root namespace")
	pid := uint32(cmd.ProcessState.Pid())

	require.Eventually(t, func() bool {
		_, capturedExec := execSet.Load(pid)
		_, capturedExit := exitSet.Load(pid)
		return capturedExec && capturedExit
	}, time.Second, time.Millisecond*200, "did not capture process EXEC/EXIT from root namespace")

	// Process in another NS
	cmdNs, err := netns.New()
	require.NoError(t, err, "could not create network namespace for process")
	defer cmdNs.Close()

	cmd = exec.Command("/bin/echo")
	require.NoError(t, kernel.WithNS(cmdNs, cmd.Run), "could not run process in other network namespace")
	pid = uint32(cmd.ProcessState.Pid())

	require.Eventually(t, func() bool {
		_, capturedExec := execSet.Load(pid)
		_, capturedExit := exitSet.Load(pid)
		return capturedExec && capturedExit
	}, time.Second, 200*time.Millisecond, "did not capture process EXEC/EXIT from other namespace")

	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exec.Get(), "events is not >= than exec")
	require.GreaterOrEqual(t, pm.tel.events.Get(), pm.tel.exit.Get(), "events is not >= than exit")
	require.NotEqual(t, int64(0), pm.tel.exec.Get())
	require.NotEqual(t, int64(0), pm.tel.exit.Get())
	require.Equal(t, int64(0), pm.tel.restart.Get())
	require.Equal(t, int64(0), pm.tel.reinitFailed.Get())
	require.Equal(t, int64(0), pm.tel.processScanFailed.Get())
	require.GreaterOrEqual(t, pm.tel.callbackExecuted.Get(), int64(1), "callback_executed")
}
