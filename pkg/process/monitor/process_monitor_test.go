// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package monitor

import (
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"go.uber.org/atomic"

	procutils "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util"
)

func initializePM(t *testing.T, pm *ProcessMonitor) {
	require.NoError(t, pm.Initialize())
	t.Cleanup(pm.Stop)
	time.Sleep(time.Millisecond * 500)
}

func registerCallback(t *testing.T, pm *ProcessMonitor, isExec bool, callback *ProcessCallback) func() {
	registrationFunc := pm.SubscribeExit
	if isExec {
		registrationFunc = pm.SubscribeExec
	}
	unsubscribe, err := registrationFunc(callback)
	require.NoError(t, err)
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
	pm := GetProcessMonitor()
	pm2 := GetProcessMonitor()

	require.Equal(t, pm, pm2)
}

func TestProcessMonitorSanity(t *testing.T) {
	pm := GetProcessMonitor()
	numberOfExecs := atomic.Int32{}
	testBinaryPath := getTestBinaryPath(t)
	registerCallback(t, pm, true, &ProcessCallback{
		Callback: func(pid int) { numberOfExecs.Inc() },
	})

	initializePM(t, pm)
	require.NoError(t, exec.Command(testBinaryPath, "test").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs.Load() > 1
	}, time.Second, time.Millisecond*200, "didn't capture exec events %d", numberOfExecs.Load())

}

func TestProcessRegisterMultipleExecCallbacks(t *testing.T) {
	pm := GetProcessMonitor()

	const iterations = 10
	counters := make([]*atomic.Int32, iterations)
	for i := 0; i < iterations; i++ {
		counters[i] = &atomic.Int32{}
		c := counters[i]
		registerCallback(t, pm, true, &ProcessCallback{
			Callback: func(pid int) { c.Inc() },
		})
	}

	initializePM(t, pm)
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

func TestProcessRegisterMultipleExitCallbacks(t *testing.T) {
	pm := GetProcessMonitor()

	const iterations = 10
	counters := make([]*atomic.Int32, iterations)
	for i := 0; i < iterations; i++ {
		counters[i] = &atomic.Int32{}
		c := counters[i]
		// Sanity subscribing a callback.
		registerCallback(t, pm, false, &ProcessCallback{
			Callback: func(pid int) { c.Inc() },
		})
	}

	initializePM(t, pm)
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

func TestProcessMonitorRefcount(t *testing.T) {
	pm := GetProcessMonitor()
	require.Equal(t, pm.refcount.Load(), int32(0))

	for i := 1; i <= 10; i++ {
		require.NoError(t, pm.Initialize())
		require.Equal(t, pm.refcount.Load(), int32(i))
	}

	for i := 1; i <= 10; i++ {
		pm.Stop()
		require.Equal(t, pm.refcount.Load(), int32(10-i))
	}
}

func TestProcessMonitorInNamespace(t *testing.T) {
	execSet := sync.Map{}

	pm := GetProcessMonitor()

	registerCallback(t, pm, true, &ProcessCallback{
		Callback: func(pid int) {
			execSet.Store(pid, struct{}{})
		},
	})

	monNs, err := netns.New()
	require.NoError(t, err, "could not create network namespace for process monitor")
	t.Cleanup(func() { monNs.Close() })

	require.NoError(t, procutils.WithNS(monNs, pm.Initialize), "could not start process monitor in netNS")
	t.Cleanup(pm.Stop)

	time.Sleep(500 * time.Millisecond)
	// Process in root NS
	cmd := exec.Command("/bin/echo")
	require.NoError(t, cmd.Run(), "could not run process in root namespace")

	require.Eventually(t, func() bool {
		_, captured := execSet.Load(cmd.ProcessState.Pid())
		return captured
	}, time.Second, time.Millisecond*200, "did not capture process EXEC from root namespace")

	// Process in another NS
	cmdNs, err := netns.New()
	require.NoError(t, err, "could not create network namespace for process")
	defer cmdNs.Close()

	cmd = exec.Command("/bin/echo")
	require.NoError(t, procutils.WithNS(cmdNs, cmd.Run), "could not run process in other network namespace")

	require.Eventually(t, func() bool {
		_, captured := execSet.Load(cmd.ProcessState.Pid())
		return captured
	}, time.Second, 200*time.Millisecond, "did not capture process EXEC from other namespace")
}
