// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package monitor

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"go.uber.org/atomic"

	procutils "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/gopsutil/process"
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
		FilterType: ANY,
		Callback: func(pid int) {
			numberOfExecs.Inc()
		},
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
			FilterType: ANY,
			Callback: func(pid int) {
				c.Inc()
			},
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
			FilterType: ANY,
			Callback: func(pid int) {
				c.Inc()
			},
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

func TestProcessRegisterNamedCallbacks(t *testing.T) {
	pm := GetProcessMonitor()

	numberOfExecs := atomic.Int32{}
	numberOfExits := atomic.Int32{}

	testBinaryPath := getTestBinaryPath(t)

	unsubscribeExec := registerCallback(t, pm, true, &ProcessCallback{
		FilterType: NAME,
		Regex:      regexp.MustCompile(path.Base(testBinaryPath)),
		Callback: func(pid int) {
			numberOfExecs.Inc()
		},
	})

	unsubscribeExit := registerCallback(t, pm, false, &ProcessCallback{
		FilterType: NAME,
		Regex:      regexp.MustCompile(path.Base(testBinaryPath)),
		Callback: func(pid int) {
			numberOfExits.Inc()
		},
	})

	initializePM(t, pm)
	require.NoError(t, exec.Command(testBinaryPath, "test").Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs.Load() == 1 && numberOfExits.Load() == 1
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture exec %d and exit %d", numberOfExecs.Load(), numberOfExits.Load()))

	unsubscribeExit()
	require.NoError(t, exec.Command(testBinaryPath).Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs.Load() == 2 && numberOfExits.Load() == 1
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture exec %d and exit %d", numberOfExecs.Load(), numberOfExits.Load()))

	unsubscribeExec()
	require.NoError(t, exec.Command(testBinaryPath).Run())
	require.Eventuallyf(t, func() bool {
		return numberOfExecs.Load() == 2 && numberOfExits.Load() == 1
	}, time.Second, time.Millisecond*200, fmt.Sprintf("didn't capture exec %d and exit %d", numberOfExecs.Load(), numberOfExits.Load()))
}

func TestProcessRegisterNameExitCallbackWithoutExec(t *testing.T) {
	pm := GetProcessMonitor()

	_, err := pm.SubscribeExit(&ProcessCallback{
		FilterType: NAME,
		Regex:      regexp.MustCompile("test"),
		Callback:   func(pid int) {},
	})
	require.Error(t, err)
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
		FilterType: ANY,
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

func TestRegisterMultipleSameCallbacks(t *testing.T) {
	pm := GetProcessMonitor()

	callback := &ProcessCallback{
		FilterType: ANY,
		Callback:   func(pid int) {},
	}
	registerCallback(t, pm, true, callback)
	_, err := pm.SubscribeExec(callback)
	require.Error(t, err)
}

func T1estProcessMonitorLoad(t *testing.T) {
	execSet := atomic.Uint32{}
	exitSet := atomic.Uint32{}
	exec2 := atomic.Uint32{}

	execM := sync.Map{}
	exit := sync.Map{}
	execM2 := sync.Map{}
	pm := GetProcessMonitor()
	unsubscribeExec, err := pm.SubscribeExec(&ProcessCallback{
		FilterType: ANY,
		Callback: func(pid int) {
			execM.Store(pid, struct{}{})
			p, err := process.NewProcess(int32(pid))
			if err != nil {
				return
			}
			name, err := p.Name()
			if err != nil {
				return
			}
			if strings.Contains(name, "curl") {
				execSet.Inc()
			}
		},
	})
	require.NoError(t, err, "could not subscribe to EXEC events")
	defer unsubscribeExec()
	unsubscribeExit, err := pm.SubscribeExit(&ProcessCallback{
		FilterType: ANY,
		Callback: func(pid int) {
			exit.Store(pid, struct{}{})
			p, err := process.NewProcess(int32(pid))
			if err != nil {
				return
			}
			name, err := p.Name()
			if err != nil {
				return
			}
			if strings.Contains(name, "curl") {
				exitSet.Inc()
			}
		},
	})
	require.NoError(t, err, "could not subscribe to Exit events")
	defer unsubscribeExit()

	require.NoError(t, pm.Initialize())

	processNum := 5000

	for j := 0; j < 7; j++ {
		wg := sync.WaitGroup{}
		wg.Add(processNum)
		for i := 0; i < processNum; i++ {
			go func() {
				defer wg.Done()
				p := exec.Command("curl", "http://localhost:8080/delay/3")
				err := p.Start()
				if err != nil {
					t.Log(err)
					return
				}
				exec2.Inc()
				execM2.Load(p.Process.Pid)
			}()
		}

		wg.Wait()

		time.Sleep(time.Second * 5)
	}

	fmt.Println(execSet.Load(), exitSet.Load(), exec2.Load())
	time.Sleep(time.Second * 30)
}
